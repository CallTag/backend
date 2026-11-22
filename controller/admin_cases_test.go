package controller

import (
	"context"
	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminCasesAuthorizationAndVersionConflict(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	config.ResetMemoryForTest()
	config.ResetStaffForTest()
	denied := httptest.NewRecorder()
	AdminCases(denied, httptest.NewRequest("GET", "/api/v1/admin/cases", nil))
	if denied.Code != 403 {
		t.Fatal(denied.Code)
	}
	ctx := context.Background()
	if err := config.CreateStaff(ctx, "queue", "a-long-test-password", "MOD", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"); err != nil {
		t.Fatal(err)
	}
	staff, _ := config.FindStaffByUsername(ctx, "queue")
	raw, session, err := config.IssueStaffSession(ctx, staff, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := config.CreateDisputeCase(ctx, model.DisputeRequest{Phone: "+6287772223344", OwnerName: "Owner", Reason: "Wrong tag"}, "test:owner")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: helper.StaffCookieName, Value: raw})
		r.Header.Set("X-CSRF-Token", session.CSRFToken)
		w := httptest.NewRecorder()
		AdminCases(w, r)
		return w
	}
	if w := request("GET", "/api/v1/admin/cases?limit=501", ""); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "/api/v1/admin/cases?type=DISPUTE", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "queue_count") {
		t.Fatal(w.Code, w.Body.String())
	}
	path := "/api/v1/admin/cases/" + c.DisputeID
	if w := request("PATCH", path, `{"version":1,"action":"ACCEPT","reason":""}`); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("PATCH", path, `{"version":1,"outcome":"REJECTED","reason":"Evidence supports report"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("PATCH", path, `{"version":1,"action":"ACCEPT","reason":"Different decision"}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
}
