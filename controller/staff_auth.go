package controller

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// staffJSON deliberately avoids the device API wildcard CORS policy.
func staffJSON(w http.ResponseWriter, status int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.APIResponse{Success: status < 400, Code: status, Message: message, Data: data, Timestamp: time.Now().UTC()})
}
func staffOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host && (u.Scheme == "https" || !config.IsProdEnv() && u.Scheme == "http")
}

// RequireStaff only trusts server-side staff sessions; a device JWT never grants staff privileges.
func RequireStaff(w http.ResponseWriter, r *http.Request, minRole string) (*model.StaffPrincipal, bool) {
	cookie, err := r.Cookie(helper.StaffCookieName)
	if err != nil {
		staffJSON(w, 403, "Sesi staf diperlukan", nil)
		return nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	principal, err := config.ResolveStaffSession(ctx, cookie.Value, time.Now().UTC())
	if err != nil || !helper.StaffRoleAllowed(principal.Role, minRole) {
		staffJSON(w, 403, "Akses staf ditolak", nil)
		return nil, false
	}
	if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
		csrf := r.Header.Get("X-CSRF-Token")
		if !staffOriginAllowed(r) || csrf == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(principal.CSRFToken)) != 1 {
			staffJSON(w, 403, "Token CSRF tidak valid", nil)
			return nil, false
		}
	}
	return &principal, true
}

type staffLoginWindow struct {
	Start time.Time
	Count int
}

var staffLoginMu sync.Mutex
var staffLoginWindows = map[string]staffLoginWindow{}

func ResetStaffLoginLimiterForTest() {
	staffLoginMu.Lock()
	defer staffLoginMu.Unlock()
	staffLoginWindows = map[string]staffLoginWindow{}
}

// RemoteAddr is the trusted transport peer. Forwarding headers cannot choose the rate-limit key.
func allowStaffLogin(r *http.Request, now time.Time) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if parsed := net.ParseIP(ip); parsed != nil {
		ip = parsed.String()
	}
	if ip == "" {
		ip = "unknown"
	}
	staffLoginMu.Lock()
	defer staffLoginMu.Unlock()
	for key, value := range staffLoginWindows {
		if !now.Before(value.Start.Add(15 * time.Minute)) {
			delete(staffLoginWindows, key)
		}
	}
	win := staffLoginWindows[ip]
	if win.Start.IsZero() {
		win.Start = now
	}
	if win.Count >= 5 {
		return false
	}
	win.Count++
	staffLoginWindows[ip] = win
	return true
}
func staffCookie(r *http.Request, value string, expires time.Time) *http.Cookie {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	local := host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	return &http.Cookie{Name: helper.StaffCookieName, Value: value, Path: "/", HttpOnly: true, Secure: config.IsProdEnv() || !local, SameSite: http.SameSiteStrictMode, Expires: expires}
}
func StaffLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		staffJSON(w, 405, "Gunakan POST", nil)
		return
	}
	if !staffOriginAllowed(r) {
		staffJSON(w, 403, "Origin ditolak", nil)
		return
	}
	if !allowStaffLogin(r, time.Now()) {
		w.Header().Set("Retry-After", "900")
		staffJSON(w, 429, "Batas login: 5 percobaan per 15 menit", nil)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTPCode string `json:"totp_code"`
		TOTP     string `json:"totp"`
	}
	if err := helper.DecodeJSON(r, &req); err != nil {
		status := 400
		if err == helper.ErrBodyTooLarge {
			status = 413
		}
		staffJSON(w, status, "JSON login tidak valid", nil)
		return
	}
	if req.TOTPCode == "" {
		req.TOTPCode = req.TOTP
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	staff, err := config.AuthenticateStaff(ctx, req.Username, req.Password, req.TOTPCode, time.Now().UTC())
	if err != nil {
		staffJSON(w, 401, "Username, kata sandi, atau kode autentikator tidak valid", nil)
		return
	}
	raw, session, err := config.IssueStaffSession(ctx, staff, time.Now().UTC())
	if err != nil {
		staffJSON(w, 503, "Penyimpanan sesi tidak tersedia", nil)
		return
	}
	if err := config.RecordAdminAudit(ctx, staff.ID, "STAFF_LOGIN", "staff_session", "Staff authenticated with password and TOTP", nil, map[string]string{"role": staff.Role}); err != nil {
		_ = config.RevokeStaffSession(ctx, raw)
		staffJSON(w, 503, "Pencatatan audit gagal", nil)
		return
	}
	http.SetCookie(w, staffCookie(r, raw, session.ExpiresAt))
	staffJSON(w, 200, "Login staf berhasil", model.StaffPrincipal{ID: staff.ID, Username: staff.Username, Role: staff.Role, CSRFToken: session.CSRFToken})
}
func StaffSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		staffJSON(w, 405, "Gunakan GET", nil)
		return
	}
	p, ok := RequireStaff(w, r, "MOD")
	if !ok {
		return
	}
	staffJSON(w, 200, "Sesi staf aktif", p)
}
func StaffLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		staffJSON(w, 405, "Gunakan POST", nil)
		return
	}
	principal, ok := RequireStaff(w, r, "MOD")
	if !ok {
		return
	}
	cookie, _ := r.Cookie(helper.StaffCookieName)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := config.RecordAdminAudit(ctx, principal.ID, "STAFF_LOGOUT", "staff_session", "Staff requested logout", nil, nil); err != nil {
		staffJSON(w, 503, "Pencatatan audit gagal", nil)
		return
	}
	if err := config.RevokeStaffSession(ctx, cookie.Value); err != nil {
		staffJSON(w, 503, "Pencabutan sesi gagal", nil)
		return
	}
	expired := staffCookie(r, "", time.Unix(1, 0))
	expired.MaxAge = -1
	http.SetCookie(w, expired)
	staffJSON(w, 200, "Logout berhasil", nil)
}
