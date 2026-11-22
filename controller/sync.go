package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

// GetSyncChangesHandler menangani GET /api/v1/sync/changes?cursor=&limit= dengan cursor opak generation:seq (T-201).
func GetSyncChangesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	q := r.URL.Query()
	cursor := strings.TrimSpace(q.Get("cursor"))
	limit := 100
	if lm := q.Get("limit"); lm != "" {
		if parsed, err := strconv.Atoi(lm); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := config.GetSyncChanges(ctx, cursor, limit)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrFullResyncRequired):
			helper.WriteJSON(w, http.StatusConflict, false, "FULL_RESYNC_REQUIRED: generasi cursor sinkronisasi telah berubah, ulangi dari awal", map[string]string{
				"error_code": "FULL_RESYNC_REQUIRED",
			})
		case errors.Is(err, config.ErrInvalidCursor):
			helper.WriteError(w, http.StatusBadRequest, err.Error())
		default:
			helper.WriteError(w, http.StatusInternalServerError, "Gagal membaca jurnal perubahan sync_changes: "+err.Error())
		}
		return
	}

	helper.WriteSuccess(w, http.StatusOK, "Jurnal perubahan sinkronisasi berhasil diambil", resp)
}

// GetOfflineBundleSync menangani GET /api/v1/sync/offline-bundle untuk sinkronisasi delta ke Room DB / SQLite di Android.
func GetOfflineBundleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	q := r.URL.Query()
	var sinceVersion int64
	if sv := q.Get("since_version"); sv != "" {
		if parsed, err := strconv.ParseInt(sv, 10, 64); err == nil && parsed >= 0 {
			sinceVersion = parsed
		}
	}

	limit := 200
	if lm := q.Get("limit"); lm != "" {
		if parsed, err := strconv.Atoi(lm); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	items, latestVer, err := config.GetTopScammersForSync(ctx, sinceVersion, limit)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal mengambil paket sinkronisasi scammer: "+err.Error())
		return
	}

	national, _, _ := config.QueryEmergency(ctx, "", "", "")

	resp := model.SpamSyncResponse{
		LatestSyncVersion: latestVer,
		Count:             len(items),
		Items:             items,
		NationalEmergency: national,
		SyncedAt:          time.Now().UTC(),
	}

	helper.WriteSuccess(w, http.StatusOK, "Paket offline-bundle berhasil diambil", resp)
}

// GetPublicStats menangani GET /api/v1/stats/public sesuai PRD Bagian 5.3.
func GetPublicStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats := config.ComputePublicStats(ctx)
	helper.WriteSuccess(w, http.StatusOK, "Statistik agregat publik berhasil diambil", stats)
}

// HealthCheck menangani GET /health dan GET /healthz untuk pemeriksaan liveness server (T-107).
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}
	mode := "in-memory-seeded"
	if config.MongoConnected {
		mode = "mongodb-connected"
	}
	helper.WriteSuccess(w, http.StatusOK, "CallTag Backend gocroot aktif", map[string]interface{}{
		"service":      "calltag-backend-gocroot",
		"version":      "1.0.0",
		"status":       "alive",
		"storage_mode": mode,
		"database":     config.GetDBName(),
	})
}

// HealthzCheck adalah alias liveness probe GET /healthz (T-107).
func HealthzCheck(w http.ResponseWriter, r *http.Request) {
	HealthCheck(w, r)
}

// ReadyzCheck menangani GET /readyz untuk memeriksa kesiapan koneksi MongoDB berbatas waktu (T-107).
func ReadyzCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	if err := config.PingDatabase(r.Context()); err != nil {
		helper.WriteJSON(w, http.StatusServiceUnavailable, false, "Layanan belum siap: koneksi database gagal ("+err.Error()+")", map[string]interface{}{
			"service":  "calltag-backend-gocroot",
			"status":   "not_ready",
			"database": config.GetDBName(),
			"app_env":  config.GetAppEnv(),
		})
		return
	}

	mode := "in-memory-seeded"
	if config.MongoConnected {
		mode = "mongodb-connected"
	}
	helper.WriteSuccess(w, http.StatusOK, "CallTag Backend siap melayani permintaan", map[string]interface{}{
		"service":      "calltag-backend-gocroot",
		"status":       "ready",
		"storage_mode": mode,
		"database":     config.GetDBName(),
		"app_env":      config.GetAppEnv(),
	})
}
