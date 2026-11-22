package controller

import (
	"embed"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
)

//go:embed adminweb/*
var adminAssets embed.FS

// Respons staf tidak memakai kebijakan CORS publik milik endpoint aplikasi.
type staffResponseWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *staffResponseWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.wrote = true
	for key := range w.Header() {
		if strings.HasPrefix(strings.ToLower(key), "access-control-") {
			w.Header().Del(key)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.ResponseWriter.WriteHeader(code)
}
func (w *staffResponseWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

// AdminRoute menyajikan shell panel publik dan API data yang memerlukan sesi staf.
func AdminRoute(out http.ResponseWriter, r *http.Request) {
	w := &staffResponseWriter{ResponseWriter: out}
	path := r.URL.Path
	if path == "/admin" || strings.HasPrefix(path, "/admin/") {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			helper.WriteError(w, 405, "Gunakan GET untuk halaman panel")
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		name := "adminweb/index.html"
		contentType := "text/html; charset=utf-8"
		if strings.HasPrefix(path, "/admin/assets/") {
			switch strings.TrimPrefix(path, "/admin/assets/") {
			case "admin.js":
				name = "adminweb/admin.js"
				contentType = "text/javascript; charset=utf-8"
			case "admin.css":
				name = "adminweb/admin.css"
				contentType = "text/css; charset=utf-8"
			default:
				http.NotFound(w, r)
				return
			}
		} else {
			switch path {
			case "/admin", "/admin/", "/admin/login", "/admin/reports", "/admin/disputes", "/admin/numbers", "/admin/verified", "/admin/emergency", "/admin/channels", "/admin/audit":
			default:
				http.NotFound(w, r)
				return
			}
		}
		body, err := fs.ReadFile(adminAssets, name)
		if err != nil {
			helper.WriteError(w, 500, "Aset panel tidak tersedia")
			return
		}
		w.Header().Set("Content-Type", contentType)
		if r.Method == http.MethodHead {
			w.WriteHeader(200)
			return
		}
		_, _ = w.Write(body)
		return
	}
	switch {
	case path == "/api/v1/admin/login" || path == "/api/v1/admin/auth/login":
		StaffLogin(w, r)
	case path == "/api/v1/admin/logout" || path == "/api/v1/admin/auth/logout":
		StaffLogout(w, r)
	case path == "/api/v1/admin/auth/session":
		StaffSession(w, r)
	case path == "/api/v1/admin/cases" || strings.HasPrefix(path, "/api/v1/admin/cases/"):
		AdminCases(w, r)
	case path == "/api/v1/admin/numbers":
		AdminNumberDirectory(w, r)
	case strings.HasPrefix(path, "/api/v1/admin/numbers/") || strings.HasPrefix(path, "/api/v1/admin/phones/"):
		AdminPhoneProfile(w, r)
	case path == "/api/v1/admin/import":
		AdminImport(w, r)
	case path == "/api/v1/admin/export":
		AdminExport(w, r)
	case path == "/api/v1/admin/audit":
		adminAudit(w, r)
	case strings.HasPrefix(path, "/api/v1/admin/verified") || strings.HasPrefix(path, "/api/v1/admin/emergency") || strings.HasPrefix(path, "/api/v1/admin/channels") || strings.HasPrefix(path, "/api/v1/admin/official-channels"):
		AdminCuration(w, r)
	default:
		if _, ok := RequireStaff(w, r, "MOD"); ok {
			helper.WriteError(w, 404, "Endpoint staf tidak ditemukan")
		}
	}
}

func adminAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireStaff(w, r, "ADMIN"); !ok {
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, 405, "Gunakan GET untuk audit")
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			helper.WriteError(w, 400, "Limit harus 1–500")
			return
		}
		limit = n
	}
	rows, err := config.ListAdminAudit(r.Context(), 500)
	if err != nil {
		helper.WriteError(w, 500, "Gagal memuat audit")
		return
	}
	from, to := time.Time{}, time.Time{}
	for key, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		if raw := r.URL.Query().Get(key); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				helper.WriteError(w, 400, "Tanggal audit harus RFC3339")
				return
			}
			*dst = parsed
		}
	}
	actor := r.URL.Query().Get("actor")
	filtered := rows[:0]
	for _, row := range rows {
		if actor != "" && row.Actor != actor {
			continue
		}
		if !from.IsZero() && row.CreatedAt.Before(from) {
			continue
		}
		if !to.IsZero() && row.CreatedAt.After(to) {
			continue
		}
		filtered = append(filtered, row)
		if len(filtered) >= limit {
			break
		}
	}
	helper.WriteSuccess(w, 200, "Audit perubahan staf", filtered)
}
