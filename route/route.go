package route

import (
	"net/http"
	"strings"

	"github.com/CallTag/backend/controller"
	"github.com/CallTag/backend/helper"
)

// URL adalah router utama bergaya gocroot yang memetakan ke-11 kontrak endpoint resmi PRD Bagian 5.3:
// 1.  GET  /health
// 2.  POST /api/v1/auth/device
// 3.  GET  /api/v1/phones/{e164}
// 4.  GET  /api/v1/phones/{e164}/reports
// 5.  POST /api/v1/reports
// 6.  POST /api/v1/disputes
// 7.  GET  /api/v1/emergency/national
// 8.  GET  /api/v1/emergency/local
// 9.  GET  /api/v1/official-channels
// 10. GET  /api/v1/sync/offline-bundle
// 11. GET  /api/v1/stats/public
func URL(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/admin" || strings.HasPrefix(r.URL.Path, "/admin/") || r.URL.Path == "/api/v1/admin" || strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
		controller.AdminRoute(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := r.URL.Path

	switch {
	case path == "/" || path == "/health" || path == "/api/v1/health":
		controller.HealthCheck(w, r)

	case path == "/healthz":
		controller.HealthzCheck(w, r)

	case path == "/readyz":
		controller.ReadyzCheck(w, r)

	case path == "/api/v1/auth/device":
		controller.AuthLogin(w, r)

	case path == "/api/v1/auth/google":
		controller.AuthGoogle(w, r)

	case path == "/api/v1/auth/refresh":
		controller.AuthRefresh(w, r)

	case path == "/api/v1/me":
		controller.MeHandler(w, r)

	case strings.HasPrefix(path, "/api/v1/phones/") && strings.HasSuffix(path, "/reports") && !strings.Contains(strings.TrimSuffix(strings.TrimPrefix(path, "/api/v1/phones/"), "/reports"), "/"):
		controller.GetPhoneReports(w, r)

	case strings.HasPrefix(path, "/api/v1/phones/") && !strings.Contains(strings.TrimPrefix(path, "/api/v1/phones/"), "/"):
		controller.LookupPhone(w, r)

	case path == "/api/v1/reports":
		controller.SubmitReport(w, r)

	case path == "/api/v1/disputes":
		controller.SubmitDispute(w, r)

	case strings.HasPrefix(path, "/api/v1/disputes/"):
		controller.GetDispute(w, r)

	case path == "/api/v1/emergency/national":
		controller.GetNationalEmergency(w, r)

	case path == "/api/v1/emergency/local":
		controller.GetEmergencyDirectory(w, r)

	case path == "/api/v1/official-channels":
		controller.GetOfficialChannels(w, r)

	case path == "/api/v1/sync/offline-bundle":
		controller.GetOfflineBundleSync(w, r)

	case path == "/api/v1/sync/changes":
		controller.GetSyncChangesHandler(w, r)

	case path == "/api/v1/stats/public":
		controller.GetPublicStats(w, r)

	default:
		helper.WriteError(w, http.StatusNotFound, "Endpoint tidak ditemukan: "+helper.MaskPhoneInPath(path))
	}
}
