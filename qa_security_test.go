package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/controller"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"github.com/CallTag/backend/route"
)

func initQATestStorage(t *testing.T, dbName string) bool {
	t.Helper()
	config.ResetMemoryForTest()
	controller.ResetStaffLoginLimiterForTest()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("HASH_SALT", "0123456789abcdef")

	mongoURI := strings.TrimSpace(os.Getenv("CALLTAG_TEST_MONGO_URI"))
	if mongoURI == "" {
		return false
	}
	t.Setenv("MONGOSTRING", mongoURI)
	t.Setenv("DB_NAME", dbName)
	config.InitDatabase()
	if !config.MongoConnected || config.MongoDB == nil {
		t.Fatalf("Gagal terhubung ke MongoDB uji di %s", mongoURI)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if config.MongoDB != nil {
			_ = config.MongoDB.Drop(ctx)
		}
		if config.MongoClient != nil {
			_ = config.MongoClient.Disconnect(ctx)
		}
		config.ResetMemoryForTest()
	})
	return true
}

func loginTestStaffHTTP(t *testing.T, username, role string) (*http.Cookie, string) {
	t.Helper()
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if err := config.CreateStaff(context.Background(), username, "StaffPassword123!", role, secret); err != nil {
		t.Fatalf("CreateStaff gagal: %v", err)
	}
	code, err := helper.StaffTOTPCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("StaffTOTPCode gagal: %v", err)
	}
	body, _ := json.Marshal(map[string]string{
		"username": username,
		"password": "StaffPassword123!",
		"totp":     code,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", bytes.NewReader(body))
	req.Host = "localhost:8080"
	req.RemoteAddr = "127.0.0.1:43210"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	route.URL(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/admin/login gagal: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == helper.StaffCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("Cookie sesi staf tidak diterbitkan saat login")
	}
	var resp struct {
		Data model.StaffPrincipal `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Gagal decode login staf: %v", err)
	}
	if resp.Data.CSRFToken == "" {
		t.Fatal("CSRF token staf kosong")
	}
	return sessionCookie, resp.Data.CSRFToken
}

func doJSONRoute(method, path string, body any, headers map[string]string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			reader = bytes.NewReader([]byte(v))
		default:
			b, _ := json.Marshal(v)
			reader = bytes.NewReader(b)
		}
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = "localhost:8080"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	route.URL(rec, req)
	return rec
}

// K-01 (NFR-001, FR-012): 11 perangkat berbeda melapor nomor sama -> ditahan SPAM dengan pending_danger_review=true,
// hanya naik menjadi DANGER_SCAM setelah moderator memberi CONFIRM_DANGER.
func TestQA_K01_ElevenReportersGateWithoutModerator(t *testing.T) {
	initQATestStorage(t, "calltag_qa_k01")
	targetPhone := "081377770001"
	targetE164 := "+6281377770001"

	var lastReportResp model.ReportResponse
	for i := 1; i <= 11; i++ {
		tok := registerInstallIDForTest(t, fmt.Sprintf("inst-k01-%02d", i), fmt.Sprintf("Pelapor %02d", i))
		rec := doJSONRoute(http.MethodPost, "/api/v1/reports", map[string]string{
			"phone":         targetPhone,
			"category_code": "APK_SNIFFING",
			"custom_tag":    "Kurir APK Palsu",
			"comment":       "Mengirim tautan undangan APK berbahaya",
			"call_type":     "WHATSAPP_CHAT",
		}, map[string]string{
			"Authorization":   "Bearer " + tok,
			"Idempotency-Key": fmt.Sprintf("idem-k01-%02d", i),
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("Laporan K-01 ke-%d gagal: %d %s", i, rec.Code, rec.Body.String())
		}
		var rr struct {
			Data model.ReportResponse `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &rr)
		lastReportResp = rr.Data
	}
	if !lastReportResp.PhoneTag.PendingDangerReview || lastReportResp.PhoneTag.ReputationLevel != model.ReputationSpam {
		t.Fatalf("K-01 FAIL: agregat ke-11 tanpa moderator wajib pending_danger_review=true dan SPAM, got level=%s pending=%v", lastReportResp.PhoneTag.ReputationLevel, lastReportResp.PhoneTag.PendingDangerReview)
	}

	recLookup := doJSONRoute(http.MethodGet, "/api/v1/phones/"+targetE164, nil, nil)
	if recLookup.Code != http.StatusOK {
		t.Fatalf("Lookup K-01 gagal: %d %s", recLookup.Code, recLookup.Body.String())
	}
	var lookup struct {
		Data model.LookupResponse `json:"data"`
	}
	if err := json.Unmarshal(recLookup.Body.Bytes(), &lookup); err != nil {
		t.Fatalf("Decode lookup K-01 gagal: %v", err)
	}
	if lookup.Data.UniqueReporters != 11 {
		t.Fatalf("Expected unique_reporters=11, got %d", lookup.Data.UniqueReporters)
	}
	if lookup.Data.RiskScore < 70 {
		t.Fatalf("Expected risk_score >= 70, got %.2f", lookup.Data.RiskScore)
	}
	if lookup.Data.ReputationLevel != model.ReputationSpam {
		t.Fatalf("K-01 FAIL: 11 akun tanpa moderator wajib tertahan SPAM (bukan DANGER_SCAM), got level=%s", lookup.Data.ReputationLevel)
	}

	// Pastikan kasus REPORT_REVIEW masuk antrean moderasi lalu konfirmasi moderator menaikkan ke DANGER_SCAM.
	modCookie, modCSRF := loginTestStaffHTTP(t, "qa-mod-k01", "MOD")
	recCases := doJSONRoute(http.MethodGet, "/api/v1/admin/cases?type=REPORT_REVIEW&status=OPEN", nil, nil, modCookie)
	if recCases.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/cases gagal: %d %s", recCases.Code, recCases.Body.String())
	}
	var queue struct {
		Data model.AdminCaseQueue `json:"data"`
	}
	_ = json.Unmarshal(recCases.Body.Bytes(), &queue)
	var targetCase *model.ModerationCase
	for i := range queue.Data.Items {
		if queue.Data.Items[i].PhoneE164 == targetE164 {
			targetCase = &queue.Data.Items[i]
			break
		}
	}
	if targetCase == nil {
		t.Fatalf("Kasus REPORT_REVIEW untuk %s tidak ditemukan di antrean moderator", targetE164)
	}

	for _, path := range []string{"/api/v1/admin/cases/" + targetCase.DisputeID, "/api/v1/admin/numbers/" + targetE164} {
		detail := doJSONRoute(http.MethodGet, path, nil, nil, modCookie)
		if detail.Code != http.StatusOK {
			t.Fatalf("Detail staf gagal: %d", detail.Code)
		}
		var detailBody struct {
			Data struct {
				Reports []model.SpamReport `json:"reports"`
			} `json:"data"`
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil || len(detailBody.Data.Reports) != 11 {
			t.Fatalf("Detail staf harus memuat 11 laporan: %v", err)
		}
		for _, report := range detailBody.Data.Reports {
			if report.ReporterUserID != "" || report.IdempotencyKey != "" {
				t.Fatal("Detail staf memuat pengenal internal pelapor atau idempotensi")
			}
		}
	}

	recDec := doJSONRoute(http.MethodPatch, "/api/v1/admin/cases/"+targetCase.DisputeID, map[string]any{
		"version": targetCase.Version,
		"action":  "CONFIRM_DANGER",
		"reason":  "Terverifikasi 11 pelapor mengirim APK phishing identik",
	}, map[string]string{
		"X-CSRF-Token": modCSRF,
	}, modCookie)
	if recDec.Code != http.StatusOK {
		t.Fatalf("PATCH CONFIRM_DANGER gagal: %d %s", recDec.Code, recDec.Body.String())
	}

	recAfter := doJSONRoute(http.MethodGet, "/api/v1/phones/"+targetE164, nil, nil)
	var after struct {
		Data model.LookupResponse `json:"data"`
	}
	_ = json.Unmarshal(recAfter.Body.Bytes(), &after)
	if after.Data.ReputationLevel != model.ReputationDangerScam {
		t.Fatalf("Setelah konfirmasi moderator wajib menjadi DANGER_SCAM, got level=%s", after.Data.ReputationLevel)
	}
	recAdminProf := doJSONRoute(http.MethodGet, "/api/v1/admin/numbers/"+targetE164, nil, nil, modCookie)
	var adminProf struct {
		Data struct {
			Phone model.PhoneTag `json:"phone"`
		} `json:"data"`
	}
	_ = json.Unmarshal(recAdminProf.Body.Bytes(), &adminProf)
	if adminProf.Data.Phone.PendingDangerReview || !adminProf.Data.Phone.ModeratorConfirmedDanger {
		t.Fatalf("Status moderasi admin setelah CONFIRM_DANGER tidak sesuai: pending=%v confirmed=%v", adminProf.Data.Phone.PendingDangerReview, adminProf.Data.Phone.ModeratorConfirmedDanger)
	}
}

// K-02 & K-04: Idempotensi POST laporan dua kali + pagination sync/changes detik yang sama (limit=1).
func TestQA_K02_K04_IdempotencyAndSyncCursorMongo(t *testing.T) {
	initQATestStorage(t, "calltag_qa_k02_k04")
	tok := registerInstallIDForTest(t, "inst-k02-01", "Pelapor K02")

	// K-02: Kunci idempotency sama -> 1 dokumen & ID identik.
	payload := map[string]string{
		"phone":         "081388881111",
		"category_code": "PINJOL_DC",
		"custom_tag":    "DC Pinjol Paksa",
		"comment":       "Mengancam sebar data",
	}
	rec1 := doJSONRoute(http.MethodPost, "/api/v1/reports", payload, map[string]string{
		"Authorization":   "Bearer " + tok,
		"Idempotency-Key": "qa-k02-key-001",
	})
	rec2 := doJSONRoute(http.MethodPost, "/api/v1/reports", payload, map[string]string{
		"Authorization":   "Bearer " + tok,
		"Idempotency-Key": "qa-k02-key-001",
	})
	if rec1.Code != http.StatusCreated || (rec2.Code != http.StatusOK && rec2.Code != http.StatusCreated) {
		t.Fatalf("Idempotency K-02 status tak terduga: r1=%d r2=%d", rec1.Code, rec2.Code)
	}
	var r1, r2 struct {
		Data model.ReportResponse `json:"data"`
	}
	_ = json.Unmarshal(rec1.Body.Bytes(), &r1)
	_ = json.Unmarshal(rec2.Body.Bytes(), &r2)
	if r1.Data.ReportID != r2.Data.ReportID || r2.Data.PhoneTag.TotalReports != 1 {
		t.Fatalf("K-02 FAIL: idempotensi membuat duplikat: id1=%s id2=%s total=%d", r1.Data.ReportID, r2.Data.ReportID, r2.Data.PhoneTag.TotalReports)
	}

	// Kunci idempotency sama tetapi payload berbeda -> 409 Conflict
	conflictPayload := map[string]string{
		"phone":         "081388882222",
		"category_code": "JUDOL",
		"custom_tag":    "Spam Judol",
	}
	recConflict := doJSONRoute(http.MethodPost, "/api/v1/reports", conflictPayload, map[string]string{
		"Authorization":   "Bearer " + tok,
		"Idempotency-Key": "qa-k02-key-001",
	})
	if recConflict.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict untuk Idempotency-Key bentrok payload, got %d", recConflict.Code)
	}

	// K-04: Dua perubahan berdekatan di detik sama dengan limit=1 harus tersinkron semua tanpa ada yang hilang.
	tok2 := registerInstallIDForTest(t, "inst-k04-01", "Pelapor K04")
	recA := doJSONRoute(http.MethodPost, "/api/v1/reports", map[string]string{
		"phone":         "081399990001",
		"category_code": "TELEMARKETING",
		"custom_tag":    "Sales KTA",
	}, map[string]string{"Authorization": "Bearer " + tok2, "Idempotency-Key": "k04-a"})
	recB := doJSONRoute(http.MethodPost, "/api/v1/reports", map[string]string{
		"phone":         "081399990002",
		"category_code": "TELEMARKETING",
		"custom_tag":    "Sales Asuransi",
	}, map[string]string{"Authorization": "Bearer " + tok2, "Idempotency-Key": "k04-b"})
	if recA.Code != http.StatusCreated || recB.Code != http.StatusCreated {
		t.Fatalf("Laporan K-04 gagal dibuat: A=%d B=%d", recA.Code, recB.Code)
	}

	seenKeys := make(map[string]bool)
	cursor := ""
	for page := 0; page < 50; page++ {
		path := "/api/v1/sync/changes?limit=1"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		recSync := doJSONRoute(http.MethodGet, path, nil, nil)
		if recSync.Code != http.StatusOK {
			t.Fatalf("GET %s gagal: %d (%s)", path, recSync.Code, recSync.Body.String())
		}
		var sc struct {
			Data model.SyncChangesResponse `json:"data"`
		}
		_ = json.Unmarshal(recSync.Body.Bytes(), &sc)
		for _, it := range sc.Data.Items {
			seenKeys[it.Key] = true
		}
		if !sc.Data.HasMore || sc.Data.NextCursor == "" || sc.Data.NextCursor == cursor {
			break
		}
		cursor = sc.Data.NextCursor
	}
	if !seenKeys["+6281399990001"] || !seenKeys["+6281399990002"] {
		t.Fatalf("K-04 FAIL: pagination limit=1 kehilangan item sync: seen=%v", seenKeys)
	}
}

// K-03 (NFR-004, FR-007): Dispute dan antrean moderasinya tetap persisten setelah simulasi restart server (disconnect + re-init DB).
func TestQA_K03_DisputeAndCasePersistAcrossRestartMongo(t *testing.T) {
	mongoURI := strings.TrimSpace(os.Getenv("CALLTAG_TEST_MONGO_URI"))
	if mongoURI == "" {
		t.Skip("Lewati uji restart server lintas-koneksi (set CALLTAG_TEST_MONGO_URI untuk mengaktifkan)")
	}
	initQATestStorage(t, "calltag_qa_k03")

	recPost := doJSONRoute(http.MethodPost, "/api/v1/disputes", map[string]string{
		"phone":       "081200001234",
		"owner_name":  "PT Logistik Nusantara Resmi",
		"reason":      "Nomor kantor cabang resmi untuk konfirmasi pengiriman",
		"contact_ref": "verifikasi@logistik.co.id",
	}, map[string]string{"X-Device-Id": "qa-k03-guest"})
	if recPost.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/disputes gagal: %d %s", recPost.Code, recPost.Body.String())
	}
	var created struct {
		Data model.DisputeResponse `json:"data"`
	}
	_ = json.Unmarshal(recPost.Body.Bytes(), &created)
	if created.Data.DisputeID == "" || created.Data.TrackingToken == "" {
		t.Fatalf("DisputeID / TrackingToken kosong: %+v", created.Data)
	}

	// Simulasi server mati lalu menyala kembali ke database MongoDB yang sama
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if config.MongoClient != nil {
		_ = config.MongoClient.Disconnect(ctx)
	}
	config.ResetMemoryForTest()
	controller.ResetStaffLoginLimiterForTest()
	config.InitDatabase()
	if !config.MongoConnected || config.MongoDB == nil {
		t.Fatal("Re-koneksi MongoDB setelah restart simulasi gagal")
	}

	recGet := doJSONRoute(http.MethodGet, "/api/v1/disputes/"+created.Data.DisputeID, nil, map[string]string{
		"X-Tracking-Token": created.Data.TrackingToken,
	})
	if recGet.Code != http.StatusOK {
		t.Fatalf("K-03 FAIL: readback dispute setelah restart gagal: %d %s", recGet.Code, recGet.Body.String())
	}
	var readBack struct {
		Data model.DisputeResponse `json:"data"`
	}
	_ = json.Unmarshal(recGet.Body.Bytes(), &readBack)
	if readBack.Data.DisputeID != created.Data.DisputeID || readBack.Data.PhoneE164 != "+6281200001234" || readBack.Data.Status != "UNDER_REVIEW" {
		t.Fatalf("K-03 FAIL: data dispute berubah setelah restart: %+v", readBack.Data)
	}

	// Verifikasi juga tetap muncul di antrean staf admin setelah restart
	modCookie, _ := loginTestStaffHTTP(t, "qa-mod-k03", "MOD")
	recQ := doJSONRoute(http.MethodGet, "/api/v1/admin/cases?type=DISPUTE", nil, nil, modCookie)
	if recQ.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/admin/cases pasca restart gagal: %d", recQ.Code)
	}
	var q struct {
		Data model.AdminCaseQueue `json:"data"`
	}
	_ = json.Unmarshal(recQ.Body.Bytes(), &q)
	found := false
	for _, c := range q.Data.Items {
		if c.DisputeID == created.Data.DisputeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("K-03 FAIL: kasus dispute %s hilang dari moderasi pasca restart", created.Data.DisputeID)
	}
}

// K-08 (FR-010) + Pemisahan peran Device JWT vs Staf Admin vs MOD vs ADMIN.
func TestQA_K08_DeviceJWTRejectedOnAdminAndRBAC(t *testing.T) {
	initQATestStorage(t, "calltag_qa_k08")
	deviceJWT := registerInstallIDForTest(t, "inst-k08-dev", "Pengguna Biasa")

	adminPaths := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/v1/admin/auth/session", nil},
		{http.MethodGet, "/api/v1/admin/cases", nil},
		{http.MethodGet, "/api/v1/admin/numbers/+6281299887766", nil},
		{http.MethodGet, "/api/v1/admin/export?kind=cases&format=csv", nil},
		{http.MethodGet, "/api/v1/admin/audit", nil},
		{http.MethodPost, "/api/v1/admin/verified", map[string]string{"phone_e164": "+628111111111"}},
	}

	for _, tc := range adminPaths {
		// 1. Tanpa autentikasi sama sekali -> 403
		recAnon := doJSONRoute(tc.method, tc.path, tc.body, nil)
		if recAnon.Code != http.StatusForbidden {
			t.Fatalf("Anon pada %s %s harus 403, got %d", tc.method, tc.path, recAnon.Code)
		}
		// 2. Membawa Device JWT sah (Bearer + mencoba sebagai cookie sesi staf) -> WAJIB 403
		recDev := doJSONRoute(tc.method, tc.path, tc.body, map[string]string{
			"Authorization": "Bearer " + deviceJWT,
		}, &http.Cookie{Name: helper.StaffCookieName, Value: deviceJWT})
		if recDev.Code != http.StatusForbidden {
			t.Fatalf("K-08 FAIL: Device JWT harus ditolak 403 di %s %s, got %d (%s)", tc.method, tc.path, recDev.Code, recDev.Body.String())
		}
	}

	// 3. Moderator tidak boleh akses endpoint khusus ADMIN (/audit, /verified, /import) -> 403
	modCookie, modCSRF := loginTestStaffHTTP(t, "qa-mod-k08", "MOD")
	recModAudit := doJSONRoute(http.MethodGet, "/api/v1/admin/audit", nil, nil, modCookie)
	if recModAudit.Code != http.StatusForbidden {
		t.Fatalf("MOD pada /api/v1/admin/audit harus 403, got %d", recModAudit.Code)
	}
	recModVerified := doJSONRoute(http.MethodPost, "/api/v1/admin/verified", map[string]string{
		"phone_e164":  "+6221500999",
		"kind":        "BUSINESS",
		"label":       "Bank Uji",
		"source_url":  "https://bank.example.test",
		"verified_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"expires_at":  time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		"reason":      "Uji RBAC MOD vs ADMIN",
	}, map[string]string{"X-CSRF-Token": modCSRF}, modCookie)
	if recModVerified.Code != http.StatusForbidden {
		t.Fatalf("MOD pada POST /api/v1/admin/verified harus 403, got %d", recModVerified.Code)
	}
}

// K-09, K-10, K-12 & T-604 Negatif Keamanan: PII masking, IDOR /disputes/{id}, CSRF/Origin, NoSQL JSON, replay refresh token, hapus akun.
func TestQA_SecurityNegativeIDOROriginAndAccountLifecycle(t *testing.T) {
	initQATestStorage(t, "calltag_qa_sec")

	// 1. Registrasi + ambil refresh token + buat laporan ber-PII (K-09 & K-10)
	regRec := doJSONRoute(http.MethodPost, "/api/v1/auth/device", map[string]string{
		"install_id":   "inst-qa-sec-01",
		"display_name": "Penguji Keamanan",
	}, nil)
	if regRec.Code != http.StatusOK {
		t.Fatalf("Auth device gagal: %d", regRec.Code)
	}
	var authResp struct {
		Data model.AuthLoginResponse `json:"data"`
	}
	_ = json.Unmarshal(regRec.Body.Bytes(), &authResp)
	if authResp.Data.RefreshToken == "" {
		t.Fatal("Refresh token kosong")
	}

	repRec := doJSONRoute(http.MethodPost, "/api/v1/reports", map[string]string{
		"phone":         "081366667777",
		"category_code": "SOCIAL_ENG",
		"custom_tag":    "WA 081234567890",
		"comment":       "Minta transfer ke rekening NIK 3201112223334445 atau email scam@jahat.id",
		"call_type":     "CELLULAR_CALL",
	}, map[string]string{"Authorization": "Bearer " + authResp.Data.Token})
	if repRec.Code != http.StatusCreated {
		t.Fatalf("Submit report K-09 gagal: %d %s", repRec.Code, repRec.Body.String())
	}

	listRec := doJSONRoute(http.MethodGet, "/api/v1/phones/+6281366667777/reports", nil, nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET /phones/+6281366667777/reports gagal: %d", listRec.Code)
	}
	bodyText := listRec.Body.String()
	if strings.Contains(bodyText, "reporter_user_id") || strings.Contains(bodyText, "3201112223334445") || strings.Contains(bodyText, "scam@jahat.id") || strings.Contains(bodyText, "081234567890") {
		t.Fatalf("K-09/K-10 FAIL: respons publik membocorkan PII/reporter: %s", bodyText)
	}

	// 2. IDOR pada GET /api/v1/disputes/{id}
	dispRec := doJSONRoute(http.MethodPost, "/api/v1/disputes", map[string]string{
		"phone":       "081366668888",
		"owner_name":  "Pemilik Nomor Asli",
		"reason":      "Rahasia dokumen sengketa internal",
		"contact_ref": "owner@domain.test",
	}, map[string]string{"X-Device-Id": "sec-dispute-dev"})
	var disp struct {
		Data model.DisputeResponse `json:"data"`
	}
	_ = json.Unmarshal(dispRec.Body.Bytes(), &disp)
	idorWrong := doJSONRoute(http.MethodGet, "/api/v1/disputes/"+disp.Data.DisputeID, nil, map[string]string{"X-Tracking-Token": "tebak-token-idor"})
	if idorWrong.Code != http.StatusForbidden {
		t.Fatalf("IDOR tracking token salah harus 403, got %d", idorWrong.Code)
	}
	idorNoTok := doJSONRoute(http.MethodGet, "/api/v1/disputes/"+disp.Data.DisputeID, nil, nil)
	if idorNoTok.Code != http.StatusOK || strings.Contains(idorNoTok.Body.String(), "Rahasia dokumen") {
		t.Fatalf("Bacaan publik tanpa X-Tracking-Token tidak boleh membocorkan reason: %d %s", idorNoTok.Code, idorNoTok.Body.String())
	}

	// 3. CSRF & Cross-Origin Origin check pada admin API + header keamanan admin
	adminCookie, adminCSRF := loginTestStaffHTTP(t, "qa-admin-sec", "ADMIN")
	recNoCSRF := doJSONRoute(http.MethodPost, "/api/v1/admin/logout", nil, nil, adminCookie)
	if recNoCSRF.Code != http.StatusForbidden {
		t.Fatalf("Admin POST tanpa CSRF wajib 403, got %d", recNoCSRF.Code)
	}
	recEvilOrigin := doJSONRoute(http.MethodPost, "/api/v1/admin/logout", nil, map[string]string{
		"X-CSRF-Token": adminCSRF,
		"Origin":       "https://penyerang.evil",
	}, adminCookie)
	if recEvilOrigin.Code != http.StatusForbidden {
		t.Fatalf("Admin POST dari Origin asing wajib 403, got %d", recEvilOrigin.Code)
	}
	if recEvilOrigin.Header().Get("Access-Control-Allow-Origin") != "" || recEvilOrigin.Header().Get("Cache-Control") != "no-store" || recEvilOrigin.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("Header keamanan admin tidak sesuai: %+v", recEvilOrigin.Header())
	}

	// 4. Payload NoSQL / malformed JSON & mask nomor di error 404 path
	recNoSQL := doJSONRoute(http.MethodPost, "/api/v1/auth/device", `{"install_id":{"$ne":""}}`, nil)
	if recNoSQL.Code != http.StatusBadRequest {
		t.Fatalf("NoSQL object injection JSON harus ditolak 400, got %d", recNoSQL.Code)
	}
	rec404 := doJSONRoute(http.MethodGet, "/api/v1/phones/+628123456789/nonexistent", nil, nil)
	if rec404.Code != http.StatusNotFound || strings.Contains(rec404.Body.String(), "+628123456789") {
		t.Fatalf("Path 404 tidak boleh membocorkan nomor penuh: status=%d body=%s", rec404.Code, rec404.Body.String())
	}

	// 5. Refresh rotation replay memicu reuse-detection (cabut token) + K-12 DELETE /api/v1/me mencabut refresh & akses sambil menjaga agregat
	recRefresh := doJSONRoute(http.MethodPost, "/api/v1/auth/refresh", map[string]string{"refresh_token": authResp.Data.RefreshToken}, nil)
	if recRefresh.Code != http.StatusOK {
		t.Fatalf("Refresh token pertama gagal: %d", recRefresh.Code)
	}
	var refreshed struct {
		Data model.AuthLoginResponse `json:"data"`
	}
	_ = json.Unmarshal(recRefresh.Body.Bytes(), &refreshed)
	recReplay := doJSONRoute(http.MethodPost, "/api/v1/auth/refresh", map[string]string{"refresh_token": authResp.Data.RefreshToken}, nil)
	if recReplay.Code != http.StatusUnauthorized {
		t.Fatalf("Replay refresh token lama wajib 401, got %d", recReplay.Code)
	}
	recRevokedAfterReuse := doJSONRoute(http.MethodGet, "/api/v1/me", nil, map[string]string{"Authorization": "Bearer " + refreshed.Data.Token})
	if recRevokedAfterReuse.Code != http.StatusUnauthorized {
		t.Fatalf("Token sesi setelah replay refresh wajib dicabut (401), got %d", recRevokedAfterReuse.Code)
	}

	// Akun baru membuat laporan lalu DELETE /api/v1/me -> access & refresh dicabut, agregat bertahan
	k12Reg := doJSONRoute(http.MethodPost, "/api/v1/auth/device", map[string]string{
		"install_id":   "inst-qa-k12-01",
		"display_name": "Akun K12",
	}, nil)
	var k12Auth struct {
		Data model.AuthLoginResponse `json:"data"`
	}
	_ = json.Unmarshal(k12Reg.Body.Bytes(), &k12Auth)
	recK12Rep := doJSONRoute(http.MethodPost, "/api/v1/reports", map[string]string{
		"phone":         "081366669999",
		"category_code": "PINJOL_DC",
		"custom_tag":    "Pinjol K12",
	}, map[string]string{"Authorization": "Bearer " + k12Auth.Data.Token})
	if recK12Rep.Code != http.StatusCreated {
		t.Fatalf("Laporan K-12 gagal: %d", recK12Rep.Code)
	}

	recDeleteMe := doJSONRoute(http.MethodDelete, "/api/v1/me", nil, map[string]string{"Authorization": "Bearer " + k12Auth.Data.Token})
	if recDeleteMe.Code != http.StatusAccepted {
		t.Fatalf("DELETE /api/v1/me wajib 202, got %d (%s)", recDeleteMe.Code, recDeleteMe.Body.String())
	}
	if recAccessAfterDel := doJSONRoute(http.MethodGet, "/api/v1/me", nil, map[string]string{"Authorization": "Bearer " + k12Auth.Data.Token}); recAccessAfterDel.Code != http.StatusUnauthorized {
		t.Fatalf("K-12 FAIL: access token akun terhapus wajib 401, got %d", recAccessAfterDel.Code)
	}
	if recRefAfterDel := doJSONRoute(http.MethodPost, "/api/v1/auth/refresh", map[string]string{"refresh_token": k12Auth.Data.RefreshToken}, nil); recRefAfterDel.Code != http.StatusUnauthorized {
		t.Fatalf("K-12 FAIL: refresh token akun terhapus wajib 401, got %d", recRefAfterDel.Code)
	}
	recAgg := doJSONRoute(http.MethodGet, "/api/v1/phones/+6281366669999", nil, nil)
	var agg struct {
		Data model.LookupResponse `json:"data"`
	}
	_ = json.Unmarshal(recAgg.Body.Bytes(), &agg)
	if !agg.Data.Found || agg.Data.TotalReports != 1 {
		t.Fatalf("K-12 FAIL: agregat laporan harus tetap bertahan setelah akun dihapus: %+v", agg.Data)
	}
}

// Suite false positive berlabel sintetis (15-RENCANA-PENGUJIAN.md Bagian 3):
// Mengukur FPR (False Positive Rate) & Recall kebijakan Auto-Block konservatif tanpa data nyata pengguna.
func TestQA_FalsePositiveLabeledSuite(t *testing.T) {
	now := time.Now().UTC()
	var (
		totalSamples int
		cleanSamples int
		whitelisted  int
		trueDanger   int
		fp, fn       int
		tp, tn       int
	)

	shouldAutoBlock := func(tag model.PhoneTag) bool {
		return !tag.IsWhitelistedVerified && tag.IsDanger && tag.ReputationLevel == model.ReputationDangerScam
	}

	// 1. 40 nomor bersih dengan variasi kebisingan realistis (0 laporan, 1-2 laporan berkepercayaan rendah, INFO_TAG saja, >365 hari)
	for i := 0; i < 40; i++ {
		phone := fmt.Sprintf("+628510000%04d", i)
		var reports []model.SpamReport
		switch i % 4 {
		case 0:
			reports = nil
		case 1:
			reports = makeSyntheticReports(phone, 2, true, model.CategoryTelemarketing, 0.5, 5, now)
		case 2:
			reports = makeSyntheticReports(phone, 6, true, model.CategoryInfoTag, 1.0, 2, now)
		case 3:
			reports = makeSyntheticReports(phone, 6, true, model.CategoryAPKSniffing, 1.2, 400, now)
		}
		tag := helper.AggregatePhoneTagWithTime(nil, reports, 1, now)
		cleanSamples++
		totalSamples++
		if shouldAutoBlock(tag) {
			fp++
			t.Errorf("False Positive pada nomor bersih #%d (%s): level=%s", i, phone, tag.ReputationLevel)
		} else {
			tn++
		}
	}

	// 2. 10 nomor bisnis/darurat terverifikasi meskipun diserang 10 laporan APK_SNIFFING terkoordinasi
	for i := 0; i < 10; i++ {
		phone := fmt.Sprintf("+62215009%03d", i)
		reports := makeSyntheticReports(phone, 10, true, model.CategoryAPKSniffing, 1.0, 1, now)
		existing := &model.PhoneTag{PhoneE164: phone, IsWhitelistedVerified: true}
		tag := helper.AggregatePhoneTagWithTime(existing, reports, 1, now)
		whitelisted++
		totalSamples++
		if shouldAutoBlock(tag) {
			fp++
			t.Errorf("False Positive pada nomor terverifikasi #%d (%s): level=%s", i, phone, tag.ReputationLevel)
		} else {
			tn++
		}
	}

	// 3. 10 kasus brigading (6-12 pelapor tanpa konfirmasi moderator) -> wajib TIDAK auto-block (bukan DANGER_SCAM)
	for i := 0; i < 10; i++ {
		phone := fmt.Sprintf("+628520000%04d", i)
		reports := makeSyntheticReports(phone, 6+(i%6), true, model.CategoryPinjolDC, 1.0, 1, now)
		tag := helper.AggregatePhoneTagWithTime(nil, reports, 1, now)
		cleanSamples++
		totalSamples++
		if !tag.PendingDangerReview || shouldAutoBlock(tag) {
			fp++
			t.Errorf("Brigading #%d lolos ke auto-block tanpa moderator: level=%s pending=%v", i, tag.ReputationLevel, tag.PendingDangerReview)
		} else {
			tn++
		}
	}

	// 4. 15 kasus DANGER_SCAM sah (sudah dikonfirmasi moderator atau official blacklisted) -> Recall 100%
	for i := 0; i < 15; i++ {
		phone := fmt.Sprintf("+628530000%04d", i)
		reports := makeSyntheticReports(phone, 6, true, model.CategoryAPKSniffing, 1.0, 1, now)
		existing := &model.PhoneTag{
			PhoneE164:                phone,
			ModeratorConfirmedDanger: i%2 == 0,
			IsOfficialBlacklisted:    i%2 == 1,
		}
		tag := helper.AggregatePhoneTagWithTime(existing, reports, 1, now)
		trueDanger++
		totalSamples++
		if !shouldAutoBlock(tag) {
			fn++
			t.Errorf("False Negative pada DANGER_SCAM sah #%d (%s): level=%s", i, phone, tag.ReputationLevel)
		} else {
			tp++
		}
	}

	fpr := float64(fp) / float64(fp+tn) * 100.0
	recall := float64(tp) / float64(tp+fn) * 100.0
	t.Logf("Synthetic False-Positive Suite: total=%d (non_danger=%d [clean=%d verified=%d], danger=%d) -> TP=%d TN=%d FP=%d FN=%d FPR=%.2f%% Recall=%.2f%%",
		totalSamples, fp+tn, cleanSamples, whitelisted, trueDanger, tp, tn, fp, fn, fpr, recall)

	if fp != 0 || fpr >= 0.5 {
		t.Fatalf("FPR melebihi target <0.5%%: FP=%d (%.2f%%)", fp, fpr)
	}
	if fn != 0 || recall < 100.0 {
		t.Fatalf("Recall DANGER_SCAM tidak 100%%: FN=%d (%.2f%%)", fn, recall)
	}
}

func percentileMs(samples []float64, pct float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	idx := int(pct*float64(len(sorted))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// T-606: Benchmark lokal reproducible melalui HTTP TCP loopback + MongoDB terisolasi (bila CALLTAG_TEST_MONGO_URI aktif).
func TestQA_T606_LocalAPIBenchmark(t *testing.T) {
	isMongo := initQATestStorage(t, "calltag_qa_bench")
	srv := httptest.NewServer(http.HandlerFunc(route.URL))
	defer srv.Close()
	client := srv.Client()

	// Registrasi 7 pelapor (3 laporan/5m per akun -> 20 laporan tanpa kena 429)
	var tokens []string
	for i := 0; i < 7; i++ {
		tok := registerInstallIDForTest(t, fmt.Sprintf("inst-bench-%02d", i), fmt.Sprintf("Bench %02d", i))
		tokens = append(tokens, tok)
	}

	// Warm-up 5 lookup agar koneksi pool & JIT cache siap
	for i := 0; i < 5; i++ {
		resp, err := client.Get(srv.URL + "/api/v1/phones/+6281299887766")
		if err != nil {
			t.Fatalf("Warmup GET gagal: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	lookupMs := make([]float64, 0, 100)
	for i := 0; i < 100; i++ {
		target := "+6281299887766"
		if i%2 == 1 {
			target = fmt.Sprintf("+628190000%04d", i)
		}
		t0 := time.Now()
		resp, err := client.Get(srv.URL + "/api/v1/phones/" + target)
		if err != nil {
			t.Fatalf("GET lookup bench gagal: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET lookup bench status=%d", resp.StatusCode)
		}
		lookupMs = append(lookupMs, float64(time.Since(t0).Microseconds())/1000.0)
	}

	reportMs := make([]float64, 0, 20)
	for i := 0; i < 20; i++ {
		tok := tokens[i/3]
		payload, _ := json.Marshal(map[string]string{
			"phone":         fmt.Sprintf("08135500%04d", i),
			"category_code": "TELEMARKETING",
			"custom_tag":    "Telemarketing Bench",
			"comment":       "Uji benchmark penulisan lokal",
			"call_type":     "CELLULAR_CALL",
		})
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/reports", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Idempotency-Key", fmt.Sprintf("bench-rep-%03d", i))
		t0 := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST report bench gagal: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("POST report bench ke-%d status=%d", i, resp.StatusCode)
		}
		reportMs = append(reportMs, float64(time.Since(t0).Microseconds())/1000.0)
	}

	syncMs := make([]float64, 0, 50)
	for i := 0; i < 50; i++ {
		t0 := time.Now()
		resp, err := client.Get(srv.URL + "/api/v1/sync/changes?limit=50")
		if err != nil {
			t.Fatalf("GET sync bench gagal: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET sync bench status=%d", resp.StatusCode)
		}
		syncMs = append(syncMs, float64(time.Since(t0).Microseconds())/1000.0)
	}

	mode := "in-memory"
	if isMongo {
		mode = "mongodb-28019"
	}
	t.Logf("BENCHMARK mode=%s lookup(n=%d): p50=%.3fms p95=%.3fms | report_write(n=%d): p50=%.3fms p95=%.3fms | sync_changes(n=%d): p50=%.3fms p95=%.3fms",
		mode,
		len(lookupMs), percentileMs(lookupMs, 0.50), percentileMs(lookupMs, 0.95),
		len(reportMs), percentileMs(reportMs, 0.50), percentileMs(reportMs, 0.95),
		len(syncMs), percentileMs(syncMs, 0.50), percentileMs(syncMs, 0.95),
	)
}
