package controller

import (
	"context"
	"fmt"
	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequireStaffTrustAndCSRF(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	config.ResetStaffForTest()
	for _, auth := range []string{"", "Bearer garbage"} {
		r := httptest.NewRequest("GET", "/api/v1/admin/cases", nil)
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		if _, ok := RequireStaff(w, r, "MOD"); ok || w.Code != 403 {
			t.Fatal("nonstaff accepted")
		}
	}
	ctx := context.Background()
	if err := config.CreateStaff(ctx, "mod", "a-long-test-password", "MOD", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"); err != nil {
		t.Fatal(err)
	}
	s, _ := config.FindStaffByUsername(ctx, "mod")
	raw, session, err := config.IssueStaffSession(ctx, s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, role, csrf string
		allowed            bool
	}{{"GET", "MOD", "", true}, {"GET", "ADMIN", "", false}, {"PATCH", "MOD", "", false}, {"PATCH", "MOD", session.CSRFToken, true}} {
		r := httptest.NewRequest(tc.method, "/api/v1/admin/cases", nil)
		r.AddCookie(&http.Cookie{Name: helper.StaffCookieName, Value: raw})
		r.Header.Set("X-CSRF-Token", tc.csrf)
		w := httptest.NewRecorder()
		if _, ok := RequireStaff(w, r, tc.role); ok != tc.allowed {
			t.Fatalf("%+v got %v status %d", tc, ok, w.Code)
		}
	}
}
func TestStaffLoginLimiterIgnoresForwardedIP(t *testing.T) {
	ResetStaffLoginLimiterForTest()
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = "192.0.2.10:1234"
		r.Header.Set("X-Forwarded-For", string(rune('a'+i)))
		ok := allowStaffLogin(r, time.Now())
		if ok != (i < 5) {
			t.Fatalf("attempt %d allowed %v", i, ok)
		}
	}
}

var _ model.StaffPrincipal

func TestStaffLoginCookieAndLogout(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	config.ResetStaffForTest()
	ResetStaffLoginLimiterForTest()
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if err := config.CreateStaff(context.Background(), "flow", "a-long-test-password", "ADMIN", secret); err != nil {
		t.Fatal(err)
	}
	code, _ := helper.StaffTOTPCode(secret, time.Now())
	payload := fmt.Sprintf(`{"username":" FLOW ","password":"a-long-test-password","totp_code":"%s"}`, code)
	r := httptest.NewRequest("POST", "http://localhost/api/v1/admin/login", strings.NewReader(payload))
	w := httptest.NewRecorder()
	StaffLogin(w, r)
	if w.Code != 200 {
		t.Fatalf("login %d: %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Secure {
		t.Fatalf("bad localhost cookie %+v", cookies)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("admin login uses public CORS")
	}
	p, err := config.ResolveStaffSession(context.Background(), cookies[0].Value, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	logout := httptest.NewRequest("POST", "http://localhost/api/v1/admin/logout", nil)
	logout.AddCookie(cookies[0])
	logout.Header.Set("X-CSRF-Token", p.CSRFToken)
	logout.Header.Set("Origin", "https://evil.example")
	denied := httptest.NewRecorder()
	StaffLogout(denied, logout)
	if denied.Code != 403 {
		t.Fatal("cross-origin accepted")
	}
	logout.Header.Set("Origin", "http://localhost")
	done := httptest.NewRecorder()
	StaffLogout(done, logout)
	if done.Code != 200 {
		t.Fatalf("logout %d: %s", done.Code, done.Body.String())
	}
	if _, err := config.ResolveStaffSession(context.Background(), cookies[0].Value, time.Now()); err == nil {
		t.Fatal("logout retained session")
	}
	t.Setenv("APP_ENV", "prod")
	if !staffCookie(r, "token", time.Now()).Secure {
		t.Fatal("production insecure cookie")
	}
}
