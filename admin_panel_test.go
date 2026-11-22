package main

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "github.com/CallTag/backend/route"
)

func TestAdminPanelPublicShellHasNoPrivateData(t *testing.T) {
 for _,path:=range []string{"/admin","/admin/login","/admin/assets/admin.js","/admin/assets/admin.css"} {
  rec:=httptest.NewRecorder(); route.URL(rec,httptest.NewRequest(http.MethodGet,path,nil))
  if rec.Code!=200 {t.Fatalf("%s: %d",path,rec.Code)}
  if rec.Header().Get("Cache-Control")!="no-store" || rec.Header().Get("X-Content-Type-Options")!="nosniff" {t.Fatalf("unsafe headers %s: %v",path,rec.Header())}
  if strings.Contains(rec.Header().Get("Content-Type"),"text/html") && !strings.Contains(rec.Header().Get("Content-Security-Policy"),"default-src 'self'") {t.Fatal("missing CSP")}
 }
}
