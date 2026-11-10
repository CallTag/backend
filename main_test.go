package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/model"
	"github.com/CallTag/backend/route"
)

func TestBackendPRDEndpoints(t *testing.T) {
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
