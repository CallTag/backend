package config

import (
	"context"
	"github.com/CallTag/backend/model"
	"testing"
	"time"
)

func TestVerifiedRequiresActiveFutureExpiryAndProvenance(t *testing.T) {
	ResetMemoryForTest()
	now := time.Now().UTC()
	valid := model.VerifiedNumber{PhoneE164: "+622155512345", Kind: "BUSINESS", DisplayLabel: "Public test business", SourceURL: "https://example.org/contact", VerifiedBy: "test", VerifiedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), Status: "ACTIVE"}
	for _, tc := range []struct {
		name   string
		status string
		expiry time.Time
		want   bool
	}{
		{"active", "ACTIVE", now.Add(time.Hour), true}, {"expired", "ACTIVE", now.Add(-time.Second), false}, {"boundary", "ACTIVE", now, false}, {"revoked", "REVOKED", now.Add(time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := valid
			item.Status = tc.status
			item.ExpiresAt = tc.expiry
			memMu.Lock()
			memVerifiedNumbers[item.PhoneE164] = item
			memMu.Unlock()
			got, err := GetActiveVerifiedNumber(context.Background(), item.PhoneE164, now)
			if err != nil || (got != nil) != tc.want {
				t.Fatalf("got %+v, %v; want active=%v", got, err, tc.want)
			}
		})
	}
	invalid := valid
	invalid.SourceURL = ""
	if _, err := UpsertVerifiedNumber(context.Background(), invalid); err == nil {
		t.Fatal("missing provenance accepted")
	}
}

func TestVerifiedUpsertIdempotentAndSync(t *testing.T) {
	ResetMemoryForTest()
	now := time.Now().UTC()
	item := model.VerifiedNumber{PhoneE164: "+622155512345", Kind: "BUSINESS", DisplayLabel: "Public test business", SourceURL: "https://example.org/contact", VerifiedBy: "test", VerifiedAt: now, ExpiresAt: now.Add(time.Hour), Status: "ACTIVE"}
	before := len(memSyncChanges)
	changed, err := UpsertVerifiedNumber(context.Background(), item)
	if err != nil || !changed {
		t.Fatalf("first upsert %v %v", changed, err)
	}
	changed, err = UpsertVerifiedNumber(context.Background(), item)
	if err != nil || changed {
		t.Fatalf("repeat upsert %v %v", changed, err)
	}
	if len(memSyncChanges) != before+1 {
		t.Fatal("idempotent import must emit one sync change")
	}
	change := memSyncChanges[len(memSyncChanges)-1]
	if change.Kind != "VERIFIED_NUMBER" {
		t.Fatalf("unexpected sync kind %+v", change)
	}
}
