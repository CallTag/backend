package controller

import (
	"bytes"
	"context"
	"fmt"
	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCSVFormulaDefense(t *testing.T) {
	for _, v := range []string{"=SUM(A1)", "+6281234", "-1", "@cmd", "  =1", "\t123", "\r123"} {
		if !strings.HasPrefix(safeCSVCell(v), "'") {
			t.Fatalf("unsafe %q", v)
		}
	}
	if safeCSVCell("Public office") != "Public office" {
		t.Fatal("ordinary text changed")
	}
}
func TestEmergencyBatchAllRowsValidated(t *testing.T) {
	stamp := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	valid := fmt.Sprintf(`{"code":"POLICE_TEST","scope":"NATIONAL","agency_type":"POLISI","name":"Public police","contacts":{"short_code":"110","phone_numbers":[]},"verified_at":%q}`, stamp)
	rows, err := parseEmergencyBatch(strings.NewReader("["+valid+"]"), ".json", "https://example.test/contact", "staff")
	if err != nil || len(rows) != 1 || rows[0].VerifiedBy != "staff" {
		t.Fatalf("valid %v %v", rows, err)
	}
	if _, err = parseEmergencyBatch(strings.NewReader("["+valid+`,{"code":"bad"}]`), ".json", "https://example.test/contact", "staff"); err == nil {
		t.Fatal("partial invalid batch accepted")
	}
	if _, err = parseEmergencyBatch(strings.NewReader("["+valid+","+valid+"]"), ".json", "https://example.test/contact", "staff"); err == nil {
		t.Fatal("duplicate code accepted")
	}
}

func TestAdminCurationRBACCSRFAndExport(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	config.ResetMemoryForTest()
	config.ResetStaffMemoryForTest()
	defer config.ResetMemoryForTest()
	ctx := context.Background()
	makeSession := func(username, role string) (string, string) {
		t.Helper()
		if err := config.CreateStaff(ctx, username, "long-enough-password", role, "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"); err != nil {
			t.Fatal(err)
		}
		staff, err := config.FindStaffByUsername(ctx, username)
		if err != nil {
			t.Fatal(err)
		}
		raw, session, err := config.IssueStaffSession(ctx, staff, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return raw, session.CSRFToken
	}
	admin, csrf := makeSession("curation-admin", "ADMIN")
	mod, _ := makeSession("curation-mod", "MOD")
	body := fmt.Sprintf(`{"phone_e164":"+622155543210","kind":"BUSINESS","display_label":"Public office","source_url":"https://example.test/contact","verified_at":%q,"expires_at":%q,"reason":"checked official source"}`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	request := func(handler http.HandlerFunc, method, path, cookie, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: helper.StaffCookieName, Value: cookie})
		}
		if token != "" {
			r.Header.Set("X-CSRF-Token", token)
		}
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	for _, s := range []struct{ cookie, token string }{{"", ""}, {mod, ""}, {admin, ""}} {
		w := request(AdminCuration, "POST", "/api/v1/admin/verified-numbers", s.cookie, s.token, body)
		if w.Code != 403 {
			t.Fatalf("unauthorized mutation %d %s", w.Code, w.Body.String())
		}
	}
	w := request(AdminCuration, "POST", "/api/v1/admin/verified-numbers", admin, csrf, body)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("staff cache/CORS policy")
	}
	w = request(AdminExport, "GET", "/api/v1/admin/export?dataset=verified_numbers&format=csv", mod, "", "")
	if w.Code != 403 {
		t.Fatal("MOD export accepted")
	}
	w = request(AdminExport, "GET", "/api/v1/admin/export?dataset=verified_numbers&format=csv", admin, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "'+622155543210") {
		t.Fatalf("safe CSV %d %s", w.Code, w.Body.String())
	}
}

func TestAdminExportEmptyCSVWritesHeaderRow(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	config.ResetMemoryForTest()
	config.ResetStaffMemoryForTest()
	defer config.ResetMemoryForTest()
	ctx := context.Background()
	if err := config.CreateStaff(ctx, "export-admin", "long-enough-password", "ADMIN", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"); err != nil {
		t.Fatal(err)
	}
	staff, err := config.FindStaffByUsername(ctx, "export-admin")
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := config.IssueStaffSession(ctx, staff, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		dataset string
		wantCol string
	}{
		{"phone_tags", "phone_e164,primary_category,reputation_level"},
		{"verified_numbers", "display_label,expires_at,kind,phone_e164"},
		{"moderation_cases", "dispute_id,phone_e164,status"},
	} {
		r := httptest.NewRequest("GET", "/api/v1/admin/export?dataset="+tc.dataset+"&format=csv", nil)
		r.AddCookie(&http.Cookie{Name: helper.StaffCookieName, Value: raw})
		w := httptest.NewRecorder()
		AdminExport(w, r)
		if w.Code != 200 {
			t.Fatalf("%s status=%d %s", tc.dataset, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), tc.wantCol) {
			t.Fatalf("%s empty CSV missing header %q, got %q", tc.dataset, tc.wantCol, w.Body.String())
		}
	}
}

func TestAdminImportEmergencyBatchHTTPUpsertsCleanly(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	config.ResetMemoryForTest()
	config.ResetStaffMemoryForTest()
	defer config.ResetMemoryForTest()
	ctx := context.Background()
	if err := config.CreateStaff(ctx, "import-admin", "long-enough-password", "ADMIN", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"); err != nil {
		t.Fatal(err)
	}
	staff, err := config.FindStaffByUsername(ctx, "import-admin")
	if err != nil {
		t.Fatal(err)
	}
	raw, session, err := config.IssueStaffSession(ctx, staff, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	payload := fmt.Sprintf(`[{"code":"ID-NAT-110","scope":"NATIONAL","agency_type":"POLISI","name":"Polri 110 Terkurasi","contacts":{"short_code":"110","phone_numbers":["110"]},"verified_at":%q},{"code":"ID-TEST-NEW","scope":"NATIONAL","agency_type":"DAMKAR","name":"Posko Damkar Baru","contacts":{"short_code":"113","phone_numbers":["113"]},"verified_at":%q}]`, stamp, stamp)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("source", "https://example.test/official-emergency")
	_ = mw.WriteField("reason", "Sinkronisasi batch direktori")
	fw, err := mw.CreateFormFile("file", "emergency.json")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(payload))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/import?kind=emergency&dry_run=false", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", session.CSRFToken)
	req.AddCookie(&http.Cookie{Name: helper.StaffCookieName, Value: raw})
	w := httptest.NewRecorder()
	AdminImport(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"written":2`) {
		t.Fatalf("emergency batch import failed: status=%d body=%s", w.Code, w.Body.String())
	}
}
