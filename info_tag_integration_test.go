package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/model"
	"github.com/CallTag/backend/route"
)

func TestInfoTagPersistsWithoutSpamRisk(t *testing.T) {
	config.ResetMemoryForTest()
	token := registerInstallIDForTest(t, "info-tag-install", "Pengguna")
	payload := `{"phone":"081112345678","category_code":"INFO_TAG","custom_tag":"Kurir 081234567890","comment":"Layanan paket"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reports", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", "info-tag-test")
	rec := httptest.NewRecorder()
	route.URL(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("INFO_TAG ditolak: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data model.ReportResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	tag := body.Data.PhoneTag
	if tag.RiskScore != 0 || tag.ReputationLevel != model.ReputationClean || len(tag.Tags) != 1 {
		t.Fatalf("tag tidak netral: %+v", tag)
	}
	if strings.Contains(tag.Tags[0].Tag, "081234567890") {
		t.Fatal("PII tag bocor")
	}
	req = httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281112345678/reports", nil)
	rec = httptest.NewRecorder()
	route.URL(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "INFO_TAG") {
		t.Fatalf("tag tidak tersimpan: %s", rec.Body.String())
	}
}
