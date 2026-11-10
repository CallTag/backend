package controller

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

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

// HealthCheck menangani GET /health untuk pemeriksaan status server.
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	mode := "in-memory-seeded"
	if config.MongoConnected {
		mode = "mongodb-connected"
	}
	helper.WriteSuccess(w, http.StatusOK, "CallTag Backend gocroot aktif", map[string]interface{}{
		"service":      "calltag-backend-gocroot",
		"version":      "1.0.0",
		"storage_mode": mode,
		"database":     config.GetDBName(),
	})
}
