package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"github.com/CallTag/backend/route"
)

func TestBackendPRDEndpoints(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("MONGOSTRING", "")
	t.Setenv("MONGODB_URI", "")
	t.Setenv("GOOGLE_CLIENT_ID", "")
	config.InitDatabase()

	// 1. Test GET /health
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	route.URL(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK {
		t.Fatalf("GET /health gagal: status %d", recHealth.Code)
	}

	// 2. Test POST /api/v1/auth/device
	loginPayload := map[string]string{
		"phone":        "081234567890",
		"display_name": "Budi Santoso",
		"device_id":    "samsung-s24-id-01",
	}
	bodyBytes, _ := json.Marshal(loginPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	route.URL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/auth/device gagal: status %d, body: %s", rec.Code, rec.Body.String())
	}

	var loginApiResp struct {
		Success bool                    `json:"success"`
		Data    model.AuthLoginResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginApiResp); err != nil {
		t.Fatalf("Gagal decode response auth/device: %v", err)
	}
	if loginApiResp.Data.Token == "" {
		t.Fatal("Token autentikasi kosong")
	}
	token := loginApiResp.Data.Token

	// 3. Test GET /api/v1/phones/{e164} pada nomor seed scammer APK_SNIFFING
	reqPhone := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281299887766", nil)
	recPhone := httptest.NewRecorder()
	route.URL(recPhone, reqPhone)

	if recPhone.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/phones/+6281299887766 gagal: status %d", recPhone.Code)
	}
	var lookupApiResp struct {
		Success bool                 `json:"success"`
		Data    model.LookupResponse `json:"data"`
	}
	_ = json.Unmarshal(recPhone.Body.Bytes(), &lookupApiResp)
	if !lookupApiResp.Data.Found || lookupApiResp.Data.ReputationLevel != model.ReputationDangerScam {
		t.Fatalf("Expected DANGER_SCAM untuk +6281299887766, got %+v", lookupApiResp.Data)
	}

	// 4. Test GET /api/v1/phones/{e164}/reports
	reqPhoneReports := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281299887766/reports", nil)
	recPhoneReports := httptest.NewRecorder()
	route.URL(recPhoneReports, reqPhoneReports)
	if recPhoneReports.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/phones/+6281299887766/reports gagal: status %d", recPhoneReports.Code)
	}

	// 5. Test POST /api/v1/reports untuk nomor baru
	reportPayload := map[string]string{
		"phone":         "081388889999",
		"category_code": "PINJOL_DC",
		"custom_tag":    "Teror DC Pinjol Ilegal Siang Malam",
		"comment":       "Mengancam sebar data kontak ke seluruh grup kantor",
		"call_type":     "WHATSAPP_CALL",
	}
	repBytes, _ := json.Marshal(reportPayload)
	reqReport := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	reqReport.Header.Set("Content-Type", "application/json")
	reqReport.Header.Set("Authorization", "Bearer "+token)
	recReport := httptest.NewRecorder()
	route.URL(recReport, reqReport)

	if recReport.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/reports gagal: status %d, body: %s", recReport.Code, recReport.Body.String())
	}

	// 6. Test POST /api/v1/disputes
	disputePayload := map[string]string{
		"phone":       "081388889999",
		"owner_name":  "PT Ekspedisi Nusantara",
		"reason":      "Nomor operasional kurir resmi perusahaan kami",
		"contact_ref": "legal@ekspedisinusantara.id",
	}
	dispBytes, _ := json.Marshal(disputePayload)
	reqDispute := httptest.NewRequest(http.MethodPost, "/api/v1/disputes", bytes.NewReader(dispBytes))
	reqDispute.Header.Set("Content-Type", "application/json")
	reqDispute.Header.Set("Authorization", "Bearer "+token)
	recDispute := httptest.NewRecorder()
	route.URL(recDispute, reqDispute)
	if recDispute.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/disputes gagal: status %d", recDispute.Code)
	}

	// 7. Test GET /api/v1/emergency/national
	reqNat := httptest.NewRequest(http.MethodGet, "/api/v1/emergency/national", nil)
	recNat := httptest.NewRecorder()
	route.URL(recNat, reqNat)
	if recNat.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/emergency/national gagal: status %d", recNat.Code)
	}

	// 8. Test GET /api/v1/emergency/local?city=Bandung
	reqEm := httptest.NewRequest(http.MethodGet, "/api/v1/emergency/local?city=Bandung", nil)
	recEm := httptest.NewRecorder()
	route.URL(recEm, reqEm)

	if recEm.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/emergency/local?city=Bandung gagal: status %d", recEm.Code)
	}
	var emApiResp struct {
		Success bool                    `json:"success"`
		Data    model.EmergencyResponse `json:"data"`
	}
	_ = json.Unmarshal(recEm.Body.Bytes(), &emApiResp)
	if len(emApiResp.Data.LocalDirectory) == 0 {
		t.Fatal("Expected direktori lokal Kota Bandung ditemukan, got 0")
	}

	// 9. Test GET /api/v1/official-channels
	reqCh := httptest.NewRequest(http.MethodGet, "/api/v1/official-channels", nil)
	recCh := httptest.NewRecorder()
	route.URL(recCh, reqCh)
	if recCh.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/official-channels gagal: status %d", recCh.Code)
	}

	// 10. Test GET /api/v1/sync/offline-bundle
	reqSync := httptest.NewRequest(http.MethodGet, "/api/v1/sync/offline-bundle?since_version=0", nil)
	recSync := httptest.NewRecorder()
	route.URL(recSync, reqSync)

	if recSync.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sync/offline-bundle gagal: status %d", recSync.Code)
	}
	var syncApiResp struct {
		Success bool                   `json:"success"`
		Data    model.SpamSyncResponse `json:"data"`
	}
	_ = json.Unmarshal(recSync.Body.Bytes(), &syncApiResp)
	if syncApiResp.Data.Count == 0 {
		t.Fatal("Expected item sinkronisasi scammer > 0, got 0")
	}

	// 11. Test GET /api/v1/stats/public
	reqStats := httptest.NewRequest(http.MethodGet, "/api/v1/stats/public", nil)
	recStats := httptest.NewRecorder()
	route.URL(recStats, reqStats)
	if recStats.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/stats/public gagal: status %d", recStats.Code)
	}
}

func registerInstallIDForTest(t *testing.T, installID, displayName string) string {
	t.Helper()
	payload := map[string]string{
		"install_id":   installID,
		"display_name": displayName,
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	route.URL(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Registrasi device (%s) gagal: status %d, body: %s", installID, rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool                    `json:"success"`
		Data    model.AuthLoginResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Gagal decode response auth/device: %v", err)
	}
	if resp.Data.Token == "" {
		t.Fatal("Token autentikasi device kosong")
	}
	return resp.Data.Token
}

func TestT09_ReportIdempotencyAndDedup(t *testing.T) {
	config.ResetMemoryForTest()

	token := registerInstallIDForTest(t, "inst-t09-001", "Pelapor T09")

	reportPayload := map[string]string{
		"phone":         "081377778888",
		"category_code": "PINJOL_DC",
		"custom_tag":    "DC Pinjol Teror",
		"comment":       "Mengancam penyebaran kontak darurat",
		"call_type":     "CELLULAR_CALL",
	}
	repBytes, _ := json.Marshal(reportPayload)

	// Kirim pertama dengan Idempotency-Key
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.Header.Set("Idempotency-Key", "idem-key-001")
	rec1 := httptest.NewRecorder()
	route.URL(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/reports ke-1 gagal: status %d, body: %s", rec1.Code, rec1.Body.String())
	}

	var r1 struct {
		Success bool                 `json:"success"`
		Data    model.ReportResponse `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &r1); err != nil {
		t.Fatalf("Gagal decode ReportResponse ke-1: %v", err)
	}
	if len(r1.Data.ReportID) != 24 {
		t.Fatalf("Expected report_id berupa ObjectID hex 24 karakter, got %q", r1.Data.ReportID)
	}
	if r1.Data.PhoneTag.TotalReports != 1 {
		t.Fatalf("Expected total_reports == 1 setelah laporan pertama, got %d", r1.Data.PhoneTag.TotalReports)
	}

	// Kirim ulang dengan Idempotency-Key yang sama
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("Idempotency-Key", "idem-key-001")
	rec2 := httptest.NewRecorder()
	route.URL(rec2, req2)
	if rec2.Code != http.StatusOK && rec2.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/reports idempotent replay gagal: status %d, body: %s", rec2.Code, rec2.Body.String())
	}

	var r2 struct {
		Success bool                 `json:"success"`
		Data    model.ReportResponse `json:"data"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &r2)
	if r2.Data.ReportID != r1.Data.ReportID {
		t.Fatalf("Replay idempotency menghasilkan report_id berbeda: %q vs %q", r1.Data.ReportID, r2.Data.ReportID)
	}
	if r2.Data.PhoneTag.TotalReports != 1 || r2.Data.PhoneTag.RiskScore != r1.Data.PhoneTag.RiskScore {
		t.Fatalf("Replay idempotency menggandakan skor/laporan: total=%d, score=%.2f vs %.2f", r2.Data.PhoneTag.TotalReports, r2.Data.PhoneTag.RiskScore, r1.Data.PhoneTag.RiskScore)
	}

	// Kirim ulang oleh user sama ke nomor sama dalam 30 hari meski Idempotency-Key berbeda
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+token)
	req3.Header.Set("Idempotency-Key", "idem-key-002")
	rec3 := httptest.NewRecorder()
	route.URL(rec3, req3)
	if rec3.Code != http.StatusOK && rec3.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/reports dedup 30 hari gagal: status %d", rec3.Code)
	}

	// Verifikasi jumlah laporan tersimpan tetap 1
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281377778888/reports", nil)
	recList := httptest.NewRecorder()
	route.URL(recList, reqList)
	var listResp struct {
		Data model.PhoneReportsResponse `json:"data"`
	}
	_ = json.Unmarshal(recList.Body.Bytes(), &listResp)
	if len(listResp.Data.Reports) != 1 || listResp.Data.TotalReports != 1 {
		t.Fatalf("Expected hanya 1 laporan tersimpan setelah dedup, got len=%d total=%d", len(listResp.Data.Reports), listResp.Data.TotalReports)
	}
}

func TestT10_AuthStrictRejection(t *testing.T) {
	config.ResetMemoryForTest()

	repBytes, _ := json.Marshal(map[string]string{
		"phone":         "081366667777",
		"category_code": "SOCIAL_ENG",
		"custom_tag":    "Penipu Hadiah",
		"comment":       "Mengaku undian berhadiah",
	})

	// 1. Guest (tanpa Authorization, hanya X-Device-Id) dilarang melapor
	reqGuest := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	reqGuest.Header.Set("Content-Type", "application/json")
	reqGuest.Header.Set("X-Device-Id", "spoofed-device-001")
	recGuest := httptest.NewRecorder()
	route.URL(recGuest, reqGuest)
	if recGuest.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 untuk laporan tanpa Bearer token, got %d (%s)", recGuest.Code, recGuest.Body.String())
	}

	// 2. Token rusak pada POST /api/v1/reports wajib 401
	reqBadReport := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	reqBadReport.Header.Set("Content-Type", "application/json")
	reqBadReport.Header.Set("Authorization", "Bearer token.rusak.palsu")
	recBadReport := httptest.NewRecorder()
	route.URL(recBadReport, reqBadReport)
	if recBadReport.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 untuk token rusak di POST /reports, got %d", recBadReport.Code)
	}

	// 3. Token rusak pada GET /api/v1/phones/{e164} wajib 401 (tidak boleh turun jadi Guest 200)
	reqBadLookup := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281299887766", nil)
	reqBadLookup.Header.Set("Authorization", "Bearer token.rusak.palsu")
	recBadLookup := httptest.NewRecorder()
	route.URL(recBadLookup, reqBadLookup)
	if recBadLookup.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 untuk token rusak di GET /phones, got %d", recBadLookup.Code)
	}

	// 4. Auth device tanpa phone, install_id, maupun device_id wajib 400
	reqEmptyAuth := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device", bytes.NewReader([]byte(`{}`)))
	reqEmptyAuth.Header.Set("Content-Type", "application/json")
	recEmptyAuth := httptest.NewRecorder()
	route.URL(recEmptyAuth, reqEmptyAuth)
	if recEmptyAuth.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 untuk auth/device kosong, got %d", recEmptyAuth.Code)
	}
}

func TestT11_RateLimitAndQuota(t *testing.T) {
	config.ResetMemoryForTest()

	token := registerInstallIDForTest(t, "inst-t11-001", "Pelapor Kuota")

	phones := []string{"081399990001", "081399990002", "081399990003"}
	for i, ph := range phones {
		b, _ := json.Marshal(map[string]string{
			"phone":         ph,
			"category_code": "TELEMARKETING",
			"custom_tag":    "Spam Iklan",
			"comment":       "Menawarkan pinjaman tanpa izin",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Device-Id", "spoof-rotated-"+ph) // Coba putar X-Device-Id, harus tetap kena batas akun JWT
		rec := httptest.NewRecorder()
		route.URL(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("Laporan ke-%d gagal: status %d, body: %s", i+1, rec.Code, rec.Body.String())
		}
	}

	// Laporan ke-4 dalam 5 menit wajib ditolak 429 Too Many Requests
	b4, _ := json.Marshal(map[string]string{
		"phone":         "081399990004",
		"category_code": "TELEMARKETING",
		"custom_tag":    "Spam Iklan",
		"comment":       "Panggilan promosi berulang",
	})
	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(b4))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("Authorization", "Bearer "+token)
	req4.Header.Set("X-Device-Id", "completely-different-device-id")
	rec4 := httptest.NewRecorder()
	route.URL(rec4, req4)
	if rec4.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected 429 saat melebihi batas 3 laporan/5 menit, got %d (%s)", rec4.Code, rec4.Body.String())
	}
}

func TestT12_DisputePersistenceAndReadback(t *testing.T) {
	config.ResetMemoryForTest()

	payload, _ := json.Marshal(map[string]string{
		"phone":       "081200000001",
		"owner_name":  "Toko Elektronik Maju",
		"reason":      "Nomor ini adalah layanan pelanggan resmi toko kami",
		"contact_ref": "cs@tokomaju.id",
	})

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/disputes", bytes.NewReader(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Device-Id", "guest-dispute-dev-01")
	rec1 := httptest.NewRecorder()
	route.URL(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/disputes gagal: status %d, body: %s", rec1.Code, rec1.Body.String())
	}

	var dResp struct {
		Success bool                  `json:"success"`
		Data    model.DisputeResponse `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &dResp); err != nil {
		t.Fatalf("Gagal decode DisputeResponse: %v", err)
	}
	if dResp.Data.DisputeID == "" || dResp.Data.TrackingToken == "" {
		t.Fatalf("Expected dispute_id dan tracking_token terisi, got %+v", dResp.Data)
	}
	if dResp.Data.PhoneE164 != "+6281200000001" || dResp.Data.Status != "UNDER_REVIEW" {
		t.Fatalf("Data dispute tidak sesuai: %+v", dResp.Data)
	}

	// Baca kembali via GET /api/v1/disputes/{id}
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/disputes/"+dResp.Data.DisputeID, nil)
	reqGet.Header.Set("X-Tracking-Token", dResp.Data.TrackingToken)
	recGet := httptest.NewRecorder()
	route.URL(recGet, reqGet)
	if recGet.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/disputes/%s gagal: status %d, body: %s", dResp.Data.DisputeID, recGet.Code, recGet.Body.String())
	}
	var readResp struct {
		Data model.DisputeResponse `json:"data"`
	}
	_ = json.Unmarshal(recGet.Body.Bytes(), &readResp)
	if readResp.Data.PhoneE164 != "+6281200000001" || readResp.Data.Status != "UNDER_REVIEW" {
		t.Fatalf("Readback dispute tidak cocok: %+v", readResp.Data)
	}

	// Ajukan sanggahan aktif ganda untuk nomor sama dari pengaju sama -> 409 Conflict
	reqDup := httptest.NewRequest(http.MethodPost, "/api/v1/disputes", bytes.NewReader(payload))
	reqDup.Header.Set("Content-Type", "application/json")
	reqDup.Header.Set("X-Device-Id", "guest-dispute-dev-01")
	recDup := httptest.NewRecorder()
	route.URL(recDup, reqDup)
	if recDup.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict untuk dispute aktif ganda, got %d (%s)", recDup.Code, recDup.Body.String())
	}

	// Baca tanpa X-Tracking-Token -> 200 dengan alasan teredaksi
	reqPublic := httptest.NewRequest(http.MethodGet, "/api/v1/disputes/"+dResp.Data.DisputeID, nil)
	recPublic := httptest.NewRecorder()
	route.URL(recPublic, reqPublic)
	if recPublic.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/disputes tanpa tracking token gagal: status %d", recPublic.Code)
	}
	var pubResp struct {
		Data model.DisputeResponse `json:"data"`
	}
	_ = json.Unmarshal(recPublic.Body.Bytes(), &pubResp)
	if pubResp.Data.Reason != "" || pubResp.Data.TrackingToken != "" {
		t.Fatalf("Expected reason & tracking_token teredaksi untuk bacaan publik, got %+v", pubResp.Data)
	}

	// Baca dengan X-Tracking-Token salah -> 403 Forbidden
	reqWrongTok := httptest.NewRequest(http.MethodGet, "/api/v1/disputes/"+dResp.Data.DisputeID, nil)
	reqWrongTok.Header.Set("X-Tracking-Token", "token-pelacak-palsu")
	recWrongTok := httptest.NewRecorder()
	route.URL(recWrongTok, reqWrongTok)
	if recWrongTok.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 Forbidden untuk X-Tracking-Token salah, got %d", recWrongTok.Code)
	}

	// Kuota dispute: maksimal 3 per 24 jam (ke-4 untuk nomor lain harus 429)
	for _, ph := range []string{"081200000002", "081200000003"} {
		b, _ := json.Marshal(map[string]string{
			"phone":       ph,
			"owner_name":  "Toko Elektronik Maju",
			"reason":      "Nomor cabang resmi",
			"contact_ref": "cs@tokomaju.id",
		})
		reqExtra := httptest.NewRequest(http.MethodPost, "/api/v1/disputes", bytes.NewReader(b))
		reqExtra.Header.Set("Content-Type", "application/json")
		reqExtra.Header.Set("X-Device-Id", "guest-dispute-dev-01")
		recExtra := httptest.NewRecorder()
		route.URL(recExtra, reqExtra)
		if recExtra.Code != http.StatusCreated {
			t.Fatalf("Dispute valid ke-%s gagal: status %d", ph, recExtra.Code)
		}
	}
	b4, _ := json.Marshal(map[string]string{
		"phone":       "081200000004",
		"owner_name":  "Toko Elektronik Maju",
		"reason":      "Melebihi batas 3 per 24 jam",
		"contact_ref": "cs@tokomaju.id",
	})
	reqDisp4 := httptest.NewRequest(http.MethodPost, "/api/v1/disputes", bytes.NewReader(b4))
	reqDisp4.Header.Set("Content-Type", "application/json")
	reqDisp4.Header.Set("X-Device-Id", "guest-dispute-dev-01")
	recDisp4 := httptest.NewRecorder()
	route.URL(recDisp4, reqDisp4)
	if recDisp4.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected 429 Too Many Requests untuk dispute ke-4/24 jam, got %d", recDisp4.Code)
	}
}

func TestT20_ReadinessProbeAndProdStrict(t *testing.T) {
	config.ResetMemoryForTest()

	// 1. /healthz selalu 200
	reqHz := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recHz := httptest.NewRecorder()
	route.URL(recHz, reqHz)
	if recHz.Code != http.StatusOK {
		t.Fatalf("GET /healthz gagal: status %d", recHz.Code)
	}

	// 2. Mode dev in-memory /readyz -> 200
	t.Setenv("APP_ENV", "dev")
	t.Setenv("MONGOSTRING", "")
	t.Setenv("MONGODB_URI", "")
	reqRzDev := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recRzDev := httptest.NewRecorder()
	route.URL(recRzDev, reqRzDev)
	if recRzDev.Code != http.StatusOK {
		t.Fatalf("GET /readyz dev in-memory gagal: status %d, body: %s", recRzDev.Code, recRzDev.Body.String())
	}

	// 3. Mode prod tanpa MongoDB tersambung -> /readyz wajib 503
	t.Setenv("APP_ENV", "prod")
	reqRzProd := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recRzProd := httptest.NewRecorder()
	route.URL(recRzProd, reqRzProd)
	if recRzProd.Code != http.StatusServiceUnavailable {
		t.Fatalf("Expected 503 pada /readyz saat prod tanpa MongoDB, got %d (%s)", recRzProd.Code, recRzProd.Body.String())
	}

	// 4. Validasi config prod fail-closed bila secret pendek/kosong
	t.Setenv("JWT_SECRET", "terlalu-pendek")
	t.Setenv("HASH_SALT", "pendek")
	t.Setenv("MONGOSTRING", "")
	if err := config.ValidateConfig(); err == nil {
		t.Fatal("Expected config.ValidateConfig() gagal di mode prod dengan secret pendek")
	}

	// 5. GenerateToken di mode prod tanpa JWT_SECRET valid wajib error (tanpa static fallback)
	t.Setenv("JWT_SECRET", "")
	if _, _, err := helper.GenerateToken("u1", "", "ProdUser", "USER", 1.0); err == nil {
		t.Fatal("Expected helper.GenerateToken gagal saat APP_ENV=prod dan JWT_SECRET kosong")
	}

	// 6. Config prod lengkap wajib lolos
	t.Setenv("MONGOSTRING", "mongodb://127.0.0.1:27017")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("HASH_SALT", "0123456789abcdef")
	if err := config.ValidateConfig(); err != nil {
		t.Fatalf("Expected config.ValidateConfig() lolos saat seluruh env prod lengkap, got %v", err)
	}
}

func TestR11_PublicReportsRedactionAndPIIFilter(t *testing.T) {
	config.ResetMemoryForTest()

	// 1. Pastikan lookup/reports nomor seed tidak membocorkan reporter_user_id
	for _, ep := range []string{"/api/v1/phones/+6281299887766", "/api/v1/phones/+6281299887766/reports"} {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		rec := httptest.NewRecorder()
		route.URL(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s gagal: status %d", ep, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "reporter_user_id") {
			t.Fatalf("Kebocoran R11: respons publik %s masih mengandung field reporter_user_id", ep)
		}
	}

	// 2. Pastikan komentar mengandung NIK/HP/Email disanitasi saat disimpan
	token := registerInstallIDForTest(t, "inst-r11-001", "Pelapor R11")
	payload, _ := json.Marshal(map[string]string{
		"phone":         "081355554444",
		"category_code": "IMPERSONATION",
		"custom_tag":    "CS Bank Palsu",
		"comment":       "Penipu NIK 3201234567890001 hubungi 081299998888 email penipu@example.com",
		"call_type":     "CELLULAR_CALL",
	})
	reqRep := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(payload))
	reqRep.Header.Set("Content-Type", "application/json")
	reqRep.Header.Set("Authorization", "Bearer "+token)
	recRep := httptest.NewRecorder()
	route.URL(recRep, reqRep)
	if recRep.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/reports R11 gagal: status %d, body: %s", recRep.Code, recRep.Body.String())
	}

	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281355554444/reports", nil)
	recList := httptest.NewRecorder()
	route.URL(recList, reqList)
	bodyStr := recList.Body.String()
	if strings.Contains(bodyStr, "reporter_user_id") {
		t.Fatal("Kebocoran R11: respons /reports masih memuat reporter_user_id")
	}
	if strings.Contains(bodyStr, "3201234567890001") || strings.Contains(bodyStr, "081299998888") || strings.Contains(bodyStr, "penipu@example.com") {
		t.Fatalf("Komentar PII tidak tersanitasi di respons publik: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "[REDACTED_NIK]") || !strings.Contains(bodyStr, "[REDACTED_PHONE]") || !strings.Contains(bodyStr, "[REDACTED_EMAIL]") {
		t.Fatalf("Expected placeholder redaksi PII di komentar, got: %s", bodyStr)
	}

	// 3. Verifikasi MaskPhone untuk log aman
	if masked := helper.MaskPhone("+6281234567005"); masked != "+62812****7005" {
		t.Fatalf("Expected MaskPhone +62812****7005, got %q", masked)
	}

	// 4. Batas panjang custom_tag > 40 dan comment > 500 wajib ditolak 400
	longTagPayload, _ := json.Marshal(map[string]string{
		"phone":         "081355551111",
		"category_code": "TELEMARKETING",
		"custom_tag":    strings.Repeat("A", 41),
		"comment":       "Tag terlalu panjang",
	})
	reqLongTag := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(longTagPayload))
	reqLongTag.Header.Set("Content-Type", "application/json")
	reqLongTag.Header.Set("Authorization", "Bearer "+token)
	recLongTag := httptest.NewRecorder()
	route.URL(recLongTag, reqLongTag)
	if recLongTag.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 saat custom_tag > 40 karakter, got %d", recLongTag.Code)
	}

	longCommentPayload, _ := json.Marshal(map[string]string{
		"phone":         "081355551112",
		"category_code": "TELEMARKETING",
		"custom_tag":    "Spam Promo",
		"comment":       strings.Repeat("B", 501),
	})
	reqLongComment := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(longCommentPayload))
	reqLongComment.Header.Set("Content-Type", "application/json")
	reqLongComment.Header.Set("Authorization", "Bearer "+token)
	recLongComment := httptest.NewRecorder()
	route.URL(recLongComment, reqLongComment)
	if recLongComment.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 saat comment > 500 karakter, got %d", recLongComment.Code)
	}
}

func TestMongoDBPersistenceWhenEnabled(t *testing.T) {
	mongoURI := strings.TrimSpace(os.Getenv("CALLTAG_TEST_MONGO_URI"))
	if mongoURI == "" {
		t.Skip("Lewati uji integrasi MongoDB langsung (set CALLTAG_TEST_MONGO_URI untuk mengaktifkan)")
	}

	config.ResetMemoryForTest()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("MONGOSTRING", mongoURI)
	t.Setenv("DB_NAME", "calltag_verify_tahap3")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("HASH_SALT", "0123456789abcdef")

	config.InitDatabase()
	if !config.MongoConnected || config.MongoDB == nil {
		t.Fatalf("Gagal terhubung ke MongoDB di %s", mongoURI)
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

	// 1. /readyz wajib 200 dengan mongodb-connected
	reqRz := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recRz := httptest.NewRecorder()
	route.URL(recRz, reqRz)
	if recRz.Code != http.StatusOK || !strings.Contains(recRz.Body.String(), "mongodb-connected") {
		t.Fatalf("Expected /readyz mongodb-connected 200, got %d: %s", recRz.Code, recRz.Body.String())
	}

	// 2. Registrasi 2 user berbasis install_id tanpa phone (uji sparse unique index phone_hash)
	token1 := registerInstallIDForTest(t, "mongo-inst-001", "Pengguna Mongo 1")
	_ = registerInstallIDForTest(t, "mongo-inst-002", "Pengguna Mongo 2")

	// 3. Idempotent POST /api/v1/reports di MongoDB
	repBytes, _ := json.Marshal(map[string]string{
		"phone":         "081344445555",
		"category_code": "APK_SNIFFING",
		"custom_tag":    "Kurir Paket APK",
		"comment":       "Kirim file undangan APK dengan NIK 3201234567890001",
		"call_type":     "WHATSAPP_CHAT",
	})
	reqR1 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	reqR1.Header.Set("Content-Type", "application/json")
	reqR1.Header.Set("Authorization", "Bearer "+token1)
	reqR1.Header.Set("Idempotency-Key", "mongo-idem-001")
	recR1 := httptest.NewRecorder()
	route.URL(recR1, reqR1)
	if recR1.Code != http.StatusCreated {
		t.Fatalf("POST /reports Mongo ke-1 gagal: %d %s", recR1.Code, recR1.Body.String())
	}
	var resp1 struct {
		Data model.ReportResponse `json:"data"`
	}
	_ = json.Unmarshal(recR1.Body.Bytes(), &resp1)

	reqR2 := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	reqR2.Header.Set("Content-Type", "application/json")
	reqR2.Header.Set("Authorization", "Bearer "+token1)
	reqR2.Header.Set("Idempotency-Key", "mongo-idem-001")
	recR2 := httptest.NewRecorder()
	route.URL(recR2, reqR2)
	var resp2 struct {
		Data model.ReportResponse `json:"data"`
	}
	_ = json.Unmarshal(recR2.Body.Bytes(), &resp2)
	if resp2.Data.ReportID != resp1.Data.ReportID || resp2.Data.PhoneTag.TotalReports != 1 {
		t.Fatalf("Idempotency MongoDB gagal: r1=%s vs r2=%s, total=%d", resp1.Data.ReportID, resp2.Data.ReportID, resp2.Data.PhoneTag.TotalReports)
	}

	// 4. Persistensi dispute ke koleksi moderation_cases di MongoDB & readback
	dispBytes, _ := json.Marshal(map[string]string{
		"phone":       "081344445555",
		"owner_name":  "Kurir Resmi Nusantara",
		"reason":      "Bukan pengirim APK palsu",
		"contact_ref": "resmi@kurir.id",
	})
	reqD := httptest.NewRequest(http.MethodPost, "/api/v1/disputes", bytes.NewReader(dispBytes))
	reqD.Header.Set("Content-Type", "application/json")
	reqD.Header.Set("Authorization", "Bearer "+token1)
	recD := httptest.NewRecorder()
	route.URL(recD, reqD)
	if recD.Code != http.StatusCreated {
		t.Fatalf("POST /disputes Mongo gagal: %d %s", recD.Code, recD.Body.String())
	}
	var dResp struct {
		Data model.DisputeResponse `json:"data"`
	}
	_ = json.Unmarshal(recD.Body.Bytes(), &dResp)

	reqDG := httptest.NewRequest(http.MethodGet, "/api/v1/disputes/"+dResp.Data.DisputeID, nil)
	reqDG.Header.Set("X-Tracking-Token", dResp.Data.TrackingToken)
	recDG := httptest.NewRecorder()
	route.URL(recDG, reqDG)
	if recDG.Code != http.StatusOK {
		t.Fatalf("GET /disputes/%s Mongo gagal: %d %s", dResp.Data.DisputeID, recDG.Code, recDG.Body.String())
	}

	// 5. Uji sync/changes, /me, dan /auth/refresh pada MongoDB
	reqSync := httptest.NewRequest(http.MethodGet, "/api/v1/sync/changes?limit=5", nil)
	recSync := httptest.NewRecorder()
	route.URL(recSync, reqSync)
	if recSync.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sync/changes Mongo gagal: %d %s", recSync.Code, recSync.Body.String())
	}
	var scResp struct {
		Data model.SyncChangesResponse `json:"data"`
	}
	_ = json.Unmarshal(recSync.Body.Bytes(), &scResp)
	if scResp.Data.Count == 0 {
		t.Fatal("Expected minimal 1 perubahan di sync_changes MongoDB")
	}
}

func makeSyntheticReports(phone string, count int, uniqueUsers bool, cat model.CategoryCode, trust float64, ageDays float64, now time.Time) []model.SpamReport {
	var out []model.SpamReport
	createdAt := now.Add(-time.Duration(ageDays * float64(24*time.Hour)))
	for i := 0; i < count; i++ {
		uid := "u_same"
		if uniqueUsers {
			uid = "u_" + string(rune('a'+i))
		}
		out = append(out, model.SpamReport{
			PhoneE164:             phone,
			ReporterUserID:        uid,
			CategoryCode:          cat,
			CustomTag:             "Tag Uji",
			ReporterTrustSnapshot: trust,
			Status:                "VALIDATED",
			CreatedAt:             createdAt,
		})
	}
	return out
}

func TestT202_ScoringTableU1ToU8(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	phone := "+6281300000100"

	// U1: 5 laporan APK_SNIFFING umur 0 -> S=80, N=5 -> tertahan SPAM (OD-03) hingga moderator konfirmasi -> DANGER_SCAM
	u1Reports := makeSyntheticReports(phone, 5, true, model.CategoryAPKSniffing, 1.0, 0, now)
	ptU1Held := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u1Reports, 1, now)
	if math.Abs(ptU1Held.RiskScore-80.0) > 0.01 || ptU1Held.UniqueReporters != 5 {
		t.Fatalf("U1 skor/pelapor tidak cocok: score=%.2f N=%d", ptU1Held.RiskScore, ptU1Held.UniqueReporters)
	}
	if ptU1Held.ReputationLevel != model.ReputationSpam || !ptU1Held.PendingDangerReview {
		t.Fatalf("U1 (OD-03) wajib tertahan SPAM dengan PendingDangerReview=true, got level=%s pending=%v", ptU1Held.ReputationLevel, ptU1Held.PendingDangerReview)
	}
	ptU1Confirmed := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone, ModeratorConfirmedDanger: true}, u1Reports, 2, now)
	if ptU1Confirmed.ReputationLevel != model.ReputationDangerScam || ptU1Confirmed.PendingDangerReview {
		t.Fatalf("U1 setelah konfirmasi moderator wajib DANGER_SCAM, got level=%s pending=%v", ptU1Confirmed.ReputationLevel, ptU1Confirmed.PendingDangerReview)
	}

	// U2: 4 laporan IMPERSONATION umur 0 -> S=57.6, N=4 -> SPAM
	u2Reports := makeSyntheticReports(phone, 4, true, model.CategoryImpersonation, 1.0, 0, now)
	ptU2 := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u2Reports, 3, now)
	if math.Abs(ptU2.RiskScore-57.6) > 0.01 || ptU2.UniqueReporters != 4 || ptU2.ReputationLevel != model.ReputationSpam {
		t.Fatalf("U2 gagal: score=%.2f N=%d level=%s", ptU2.RiskScore, ptU2.UniqueReporters, ptU2.ReputationLevel)
	}

	// U3: 3 laporan PINJOL_DC umur 90 hari -> S=18.0, N=3 -> SUSPICIOUS
	u3Reports := makeSyntheticReports(phone, 3, true, model.CategoryPinjolDC, 1.0, 90, now)
	ptU3 := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u3Reports, 4, now)
	if math.Abs(ptU3.RiskScore-18.0) > 0.01 || ptU3.UniqueReporters != 3 || ptU3.ReputationLevel != model.ReputationSuspicious {
		t.Fatalf("U3 gagal: score=%.2f N=%d level=%s", ptU3.RiskScore, ptU3.UniqueReporters, ptU3.ReputationLevel)
	}

	// U4: 1 laporan APK_SNIFFING umur 0 -> S=16.0, N=1 -> CLEAN (dalam pengamatan, tidak SUSPICIOUS)
	u4Reports := makeSyntheticReports(phone, 1, true, model.CategoryAPKSniffing, 1.0, 0, now)
	ptU4 := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u4Reports, 5, now)
	if math.Abs(ptU4.RiskScore-16.0) > 0.01 || ptU4.UniqueReporters != 1 || ptU4.ReputationLevel != model.ReputationClean {
		t.Fatalf("U4 gagal: expected CLEAN untuk N=1, got score=%.2f N=%d level=%s", ptU4.RiskScore, ptU4.UniqueReporters, ptU4.ReputationLevel)
	}

	// U5: 2 laporan TELEMARKETING umur 0 -> S=16.0, N=2 -> SUSPICIOUS
	u5Reports := makeSyntheticReports(phone, 2, true, model.CategoryTelemarketing, 1.0, 0, now)
	ptU5 := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u5Reports, 6, now)
	if math.Abs(ptU5.RiskScore-16.0) > 0.01 || ptU5.UniqueReporters != 2 || ptU5.ReputationLevel != model.ReputationSuspicious {
		t.Fatalf("U5 gagal: score=%.2f N=%d level=%s", ptU5.RiskScore, ptU5.UniqueReporters, ptU5.ReputationLevel)
	}

	// U6: 1 pelapor mengirim 3 laporan APK_SNIFFING -> kontribusi maksimal 16.0, N=1 -> CLEAN
	u6Reports := makeSyntheticReports(phone, 3, false, model.CategoryAPKSniffing, 1.0, 0, now)
	ptU6 := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u6Reports, 7, now)
	if math.Abs(ptU6.RiskScore-16.0) > 0.01 || ptU6.UniqueReporters != 1 || ptU6.ReputationLevel != model.ReputationClean {
		t.Fatalf("U6 gagal: expected max per pelapor 16.0 N=1 CLEAN, got score=%.2f N=%d level=%s", ptU6.RiskScore, ptU6.UniqueReporters, ptU6.ReputationLevel)
	}

	// U7: batas 14.99 (CLEAN) vs 15.00 (SUSPICIOUS) untuk 2 pelapor
	// 2 laporan TELEMARKETING (B=8, W=1): 2 * 8 * 0.936875 = 14.99 vs 2 * 8 * 0.9375 = 15.00
	u7Below := makeSyntheticReports(phone, 2, true, model.CategoryTelemarketing, 0.936875, 0, now)
	ptU7Below := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u7Below, 8, now)
	if math.Abs(ptU7Below.RiskScore-14.99) > 0.001 || ptU7Below.ReputationLevel != model.ReputationClean {
		t.Fatalf("U7 bawah gagal: expected 14.99 CLEAN, got %.2f %s", ptU7Below.RiskScore, ptU7Below.ReputationLevel)
	}
	u7Exact := makeSyntheticReports(phone, 2, true, model.CategoryTelemarketing, 0.9375, 0, now)
	ptU7Exact := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u7Exact, 9, now)
	if math.Abs(ptU7Exact.RiskScore-15.00) > 0.001 || ptU7Exact.ReputationLevel != model.ReputationSuspicious {
		t.Fatalf("U7 batas gagal: expected 15.00 SUSPICIOUS, got %.2f %s", ptU7Exact.RiskScore, ptU7Exact.ReputationLevel)
	}

	// U8: 5 laporan APK_SNIFFING umur 180 hari -> 5 * (16 * 0.25) = 20.0 -> SUSPICIOUS
	u8Reports := makeSyntheticReports(phone, 5, true, model.CategoryAPKSniffing, 1.0, 180, now)
	ptU8 := helper.AggregatePhoneTagWithTime(&model.PhoneTag{PhoneE164: phone}, u8Reports, 10, now)
	if math.Abs(ptU8.RiskScore-20.0) > 0.01 || ptU8.UniqueReporters != 5 || ptU8.ReputationLevel != model.ReputationSuspicious {
		t.Fatalf("U8 gagal: expected 20.0 SUSPICIOUS, got %.2f %s", ptU8.RiskScore, ptU8.ReputationLevel)
	}

	// Tambahan: clamp trust_weight [0.25..1.5]
	if got := helper.ClampTrustWeight(0.05); got != 0.25 {
		t.Fatalf("Clamp bawah gagal: expected 0.25, got %.2f", got)
	}
	if got := helper.ClampTrustWeight(3.0); got != 1.5 {
		t.Fatalf("Clamp atas gagal: expected 1.5, got %.2f", got)
	}
}

func TestT201_SyncChangesJournalAndPagination(t *testing.T) {
	config.ResetMemoryForTest()
	token := registerInstallIDForTest(t, "inst-t201-001", "Pelapor Sync")

	// Ambil cursor awal sebelum menambah laporan baru
	reqInit := httptest.NewRequest(http.MethodGet, "/api/v1/sync/changes?limit=500", nil)
	recInit := httptest.NewRecorder()
	route.URL(recInit, reqInit)
	if recInit.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/sync/changes awal gagal: %d %s", recInit.Code, recInit.Body.String())
	}
	var initResp struct {
		Data model.SyncChangesResponse `json:"data"`
	}
	_ = json.Unmarshal(recInit.Body.Bytes(), &initResp)
	cursor := initResp.Data.NextCursor
	if cursor == "" {
		t.Fatal("Expected next_cursor awal terisi")
	}

	// Tambah 2 perubahan berurutan di detik yang sama
	targets := []string{"081312340001", "081312340002"}
	for _, ph := range targets {
		b, _ := json.Marshal(map[string]string{
			"phone":         ph,
			"category_code": "PINJOL_DC",
			"custom_tag":    "Tag Sync",
			"comment":       "Uji jurnal sync_changes",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		route.URL(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("Gagal buat laporan sync untuk %s: %d", ph, rec.Code)
		}
	}

	// Paginasi dengan limit=1: keduanya wajib terbaca tanpa terlewat (K-04 / T15)
	seenKeys := map[string]bool{}
	for steps := 0; steps < 10; steps++ {
		reqPage := httptest.NewRequest(http.MethodGet, "/api/v1/sync/changes?limit=1&cursor="+cursor, nil)
		recPage := httptest.NewRecorder()
		route.URL(recPage, reqPage)
		if recPage.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/sync/changes halaman ke-%d gagal: %d %s", steps+1, recPage.Code, recPage.Body.String())
		}
		var pageResp struct {
			Data model.SyncChangesResponse `json:"data"`
		}
		_ = json.Unmarshal(recPage.Body.Bytes(), &pageResp)
		for _, item := range pageResp.Data.Items {
			seenKeys[item.Key] = true
		}
		cursor = pageResp.Data.NextCursor
		if !pageResp.Data.HasMore {
			break
		}
	}
	if !seenKeys["+6281312340001"] || !seenKeys["+6281312340002"] {
		t.Fatalf("Terdapat pembaruan yang terlewat saat sinkronisasi limit=1: %+v", seenKeys)
	}

	// Cursor generasi berbeda wajib mengembalikan 409 FULL_RESYNC_REQUIRED
	reqGen := httptest.NewRequest(http.MethodGet, "/api/v1/sync/changes?cursor=99:1&limit=10", nil)
	recGen := httptest.NewRecorder()
	route.URL(recGen, reqGen)
	if recGen.Code != http.StatusConflict || !strings.Contains(recGen.Body.String(), "FULL_RESYNC_REQUIRED") {
		t.Fatalf("Expected 409 FULL_RESYNC_REQUIRED untuk generasi cursor usang, got %d (%s)", recGen.Code, recGen.Body.String())
	}
}

func TestT204_AuthRefreshAndAccountLifecycle(t *testing.T) {
	config.ResetMemoryForTest()

	// 1. Login harus mengembalikan access token dan refresh_token
	bLogin, _ := json.Marshal(map[string]string{
		"install_id":   "inst-t204-001",
		"display_name": "Akun Lifecycle",
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device", bytes.NewReader(bLogin))
	reqLogin.Header.Set("Content-Type", "application/json")
	recLogin := httptest.NewRecorder()
	route.URL(recLogin, reqLogin)
	if recLogin.Code != http.StatusOK {
		t.Fatalf("Login gagal: %d", recLogin.Code)
	}
	var loginResp struct {
		Data model.AuthLoginResponse `json:"data"`
	}
	_ = json.Unmarshal(recLogin.Body.Bytes(), &loginResp)
	if loginResp.Data.RefreshToken == "" || loginResp.Data.AccessToken == "" {
		t.Fatalf("Expected access_token & refresh_token terisi, got %+v", loginResp.Data)
	}

	// 2. GET /api/v1/me menampilkan profil & sisa kuota
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	recMe := httptest.NewRecorder()
	route.URL(recMe, reqMe)
	if recMe.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/me gagal: %d %s", recMe.Code, recMe.Body.String())
	}
	var meResp struct {
		Data model.MeResponse `json:"data"`
	}
	_ = json.Unmarshal(recMe.Body.Bytes(), &meResp)
	if meResp.Data.QuotaRemaining.Reports24hRemaining != 10 || meResp.Data.QuotaRemaining.Reports5mRemaining != 3 {
		t.Fatalf("Kuota awal tidak sesuai: %+v", meResp.Data.QuotaRemaining)
	}

	// 3. POST /api/v1/auth/refresh melakukan rotasi token
	bRefresh, _ := json.Marshal(model.RefreshRequest{RefreshToken: loginResp.Data.RefreshToken})
	reqRef := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(bRefresh))
	reqRef.Header.Set("Content-Type", "application/json")
	recRef := httptest.NewRecorder()
	route.URL(recRef, reqRef)
	if recRef.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/auth/refresh gagal: %d %s", recRef.Code, recRef.Body.String())
	}
	var refResp struct {
		Data model.AuthLoginResponse `json:"data"`
	}
	_ = json.Unmarshal(recRef.Body.Bytes(), &refResp)
	if refResp.Data.RefreshToken == "" || refResp.Data.RefreshToken == loginResp.Data.RefreshToken {
		t.Fatal("Refresh token tidak dirotasi menjadi nilai baru")
	}

	// Replay refresh_token lama wajib ditolak 401
	reqReplay := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(bRefresh))
	reqReplay.Header.Set("Content-Type", "application/json")
	recReplay := httptest.NewRecorder()
	route.URL(recReplay, reqReplay)
	if recReplay.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 saat refresh token lama dipakai ulang, got %d", recReplay.Code)
	}

	// Catatan: reuse detection mencabut sesi, jadi login kembali untuk menguji laporan + DELETE /me
	tokenActive := registerInstallIDForTest(t, "inst-t204-002", "Akun Akan Dihapus")
	repBytes, _ := json.Marshal(map[string]string{
		"phone":         "081355560001",
		"category_code": "PINJOL_DC",
		"custom_tag":    "DC Sebelum Hapus",
		"comment":       "Laporan valid sebelum hapus akun",
	})
	reqRep := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	reqRep.Header.Set("Content-Type", "application/json")
	reqRep.Header.Set("Authorization", "Bearer "+tokenActive)
	recRep := httptest.NewRecorder()
	route.URL(recRep, reqRep)
	if recRep.Code != http.StatusCreated {
		t.Fatalf("Laporan sebelum DELETE /me gagal: %d", recRep.Code)
	}

	// 4. DELETE /api/v1/me mengembalikan 202 dan menganonimkan laporan tanpa menghapus agregat (K-12)
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/me", nil)
	reqDel.Header.Set("Authorization", "Bearer "+tokenActive)
	recDel := httptest.NewRecorder()
	route.URL(recDel, reqDel)
	if recDel.Code != http.StatusAccepted {
		t.Fatalf("Expected 202 Accepted dari DELETE /api/v1/me, got %d (%s)", recDel.Code, recDel.Body.String())
	}

	// Token akun terhapus wajib ditolak 401 pada /api/v1/me
	reqMeDeleted := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	reqMeDeleted.Header.Set("Authorization", "Bearer "+tokenActive)
	recMeDeleted := httptest.NewRecorder()
	route.URL(recMeDeleted, reqMeDeleted)
	if recMeDeleted.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 untuk akun yang sudah DELETE /me, got %d", recMeDeleted.Code)
	}

	// Agregat nomor tetap ada setelah akun dihapus
	reqLookup := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281355560001", nil)
	recLookup := httptest.NewRecorder()
	route.URL(recLookup, reqLookup)
	var lookupResp struct {
		Data model.LookupResponse `json:"data"`
	}
	_ = json.Unmarshal(recLookup.Body.Bytes(), &lookupResp)
	if !lookupResp.Data.Found || lookupResp.Data.TotalReports != 1 {
		t.Fatalf("Agregat nomor tidak boleh hilang setelah DELETE /me, got %+v", lookupResp.Data)
	}
}

func TestT203_T205_T206_MiddlewareDecayAndReconcile(t *testing.T) {
	config.ResetMemoryForTest()
	token := registerInstallIDForTest(t, "inst-t205-001", "Pelapor Reconcile")

	// 1. Batas body > 16 KB ditolak 413/400 (T-203)
	oversized := `{"phone":"081311112222","category_code":"PINJOL_DC","comment":"` + strings.Repeat("x", 17*1024) + `"}`
	reqBig := httptest.NewRequest(http.MethodPost, "/api/v1/reports", strings.NewReader(oversized))
	reqBig.Header.Set("Content-Type", "application/json")
	reqBig.Header.Set("Authorization", "Bearer "+token)
	recBig := httptest.NewRecorder()
	route.URL(recBig, reqBig)
	if recBig.Code != http.StatusRequestEntityTooLarge && recBig.Code != http.StatusBadRequest {
		t.Fatalf("Expected 413/400 untuk payload > 16 KB, got %d", recBig.Code)
	}

	// MaskPhoneInPath menyamarkan nomor pada URL path log
	if masked := helper.MaskPhoneInPath("/api/v1/phones/+6281234567005/reports"); masked != "/api/v1/phones/+62812****7005/reports" {
		t.Fatalf("MaskPhoneInPath tidak sesuai: %q", masked)
	}

	// 2. Uji peluruhan/kadaluwarsa > 12 bulan via ReconcileAllPhoneTags (T-205 & T-206)
	repBytes, _ := json.Marshal(map[string]string{
		"phone":         "081311112222",
		"category_code": "PINJOL_DC",
		"custom_tag":    "Tag Kedaluwarsa",
		"comment":       "Akan kedaluwarsa dalam 400 hari",
	})
	reqRep := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewReader(repBytes))
	reqRep.Header.Set("Content-Type", "application/json")
	reqRep.Header.Set("Authorization", "Bearer "+token)
	recRep := httptest.NewRecorder()
	route.URL(recRep, reqRep)
	if recRep.Code != http.StatusCreated {
		t.Fatalf("Gagal kirim laporan untuk uji decay: %d", recRep.Code)
	}

	futureNow := time.Now().UTC().Add(400 * 24 * time.Hour)
	updated, err := config.ReconcileAllPhoneTags(context.Background(), futureNow)
	if err != nil || updated == 0 {
		t.Fatalf("ReconcileAllPhoneTags gagal: updated=%d err=%v", updated, err)
	}

	// Pastikan skor luruh menjadi 0 (CLEAN) dan jurnal DELETE tercatat
	reqAfter := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281311112222", nil)
	recAfter := httptest.NewRecorder()
	route.URL(recAfter, reqAfter)
	var afterResp struct {
		Data model.LookupResponse `json:"data"`
	}
	_ = json.Unmarshal(recAfter.Body.Bytes(), &afterResp)
	if afterResp.Data.RiskScore != 0 || afterResp.Data.ReputationLevel != model.ReputationClean {
		t.Fatalf("Expected skor 0 & CLEAN setelah > 12 bulan, got score=%.2f level=%s", afterResp.Data.RiskScore, afterResp.Data.ReputationLevel)
	}

	reqSync := httptest.NewRequest(http.MethodGet, "/api/v1/sync/changes?limit=500", nil)
	recSync := httptest.NewRecorder()
	route.URL(recSync, reqSync)
	var scResp struct {
		Data model.SyncChangesResponse `json:"data"`
	}
	_ = json.Unmarshal(recSync.Body.Bytes(), &scResp)
	foundDelete := false
	for _, it := range scResp.Data.Items {
		if it.Key == "+6281311112222" && it.Op == "DELETE" {
			foundDelete = true
			break
		}
	}
	if !foundDelete {
		t.Fatal("Expected entri DELETE di sync_changes setelah nomor kedaluwarsa > 12 bulan")
	}
}
