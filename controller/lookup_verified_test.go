package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/model"
)

func TestLookupVerifiedSeedIncludesProvenanceWithoutCommunityReports(t *testing.T) {
	config.ResetMemoryForTest()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/phones/08111500998", nil)
	rec := httptest.NewRecorder()
	LookupPhone(rec, req)
	var result struct {
		Data model.LookupResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !result.Data.Found || !result.Data.IsWhitelistedVerified || result.Data.VerifiedLabel != "OFFICIAL" || result.Data.TopCommunityLabel != "WhatsApp Halo BCA" || result.Data.VerifiedSourceURL != "https://www.bca.co.id/halobca" || result.Data.VerifiedAt == nil {
		t.Fatalf("missing verified result: %s", rec.Body.String())
	}
}

func TestLookupExpiredAndRevokedNumbersDoNotGetShield(t *testing.T) {
	config.ResetMemoryForTest()
	for _, status := range []string{"ACTIVE", "REVOKED"} {
		now := time.Now().UTC()
		expiry := now.Add(time.Hour)
		if status == "ACTIVE" {
			expiry = now.Add(-time.Hour)
		}
		item := model.VerifiedNumber{PhoneE164: "+622155512345", Kind: "BUSINESS", DisplayLabel: "Expired business", SourceURL: "https://example.org/contact", VerifiedBy: "test", VerifiedAt: now.Add(-24 * time.Hour), ExpiresAt: expiry, Status: status}
		if _, err := config.UpsertVerifiedNumber(context.Background(), item); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+622155512345", nil)
		rec := httptest.NewRecorder()
		LookupPhone(rec, req)
		var result struct {
			Data model.LookupResponse `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &result)
		if rec.Code != 200 || result.Data.IsWhitelistedVerified || result.Data.VerifiedLabel != "" {
			t.Fatalf("invalid shield: %s", rec.Body.String())
		}
	}
}

func TestLookupActiveShieldOverridesDangerButRetainsCommunityEvidence(t *testing.T) {
	config.ResetMemoryForTest()
	now := time.Now().UTC()
	item := model.VerifiedNumber{PhoneE164: "+6281299887766", Kind: "BUSINESS", DisplayLabel: "Public test service", SourceURL: "https://example.org/contact", VerifiedBy: "test", VerifiedAt: now.Add(-24 * time.Hour), ExpiresAt: now.Add(time.Hour), Status: "ACTIVE"}
	if _, err := config.UpsertVerifiedNumber(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	lookup := func() model.LookupResponse {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/phones/+6281299887766", nil)
		rec := httptest.NewRecorder()
		LookupPhone(rec, req)
		var result struct {
			Data model.LookupResponse `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if rec.Code != 200 {
			t.Fatalf("lookup: %s", rec.Body.String())
		}
		return result.Data
	}
	active := lookup()
	if !active.IsWhitelistedVerified || active.IsDanger || active.RiskScore != 0 || active.ReputationLevel != model.ReputationClean || active.TotalReports == 0 {
		t.Fatalf("shield/evidence %+v", active)
	}
	item.ExpiresAt = now.Add(-time.Hour)
	if _, err := config.UpsertVerifiedNumber(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	expired := lookup()
	if expired.IsWhitelistedVerified || !expired.IsDanger || expired.RiskScore == 0 {
		t.Fatalf("expired shield persisted %+v", expired)
	}
}
