package config

import (
	"context"
	"encoding/json"
	"github.com/CallTag/backend/internal/dataset"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"strings"
	"testing"
	"time"
)

func curationFixture() map[string]interface{} {
	return map[string]interface{}{"phone_e164": "+622155543210", "kind": "BUSINESS", "display_label": "Public office", "source_url": "https://example.test/contact", "verified_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), "expires_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "status": "ACTIVE"}
}
func testCurationLifecycle(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	fields := curationFixture()
	phone := fields["phone_e164"].(string)
	if _, err := MutateAdminCuration(ctx, "verified", "", "POST", "staff", "", fields); err == nil {
		t.Fatal("reason required")
	}
	row, err := MutateAdminCuration(ctx, "verified", "", "POST", "staff-id", "verified source", fields)
	if err != nil {
		t.Fatal(err)
	}
	if row["verified_by"] != "staff-id" {
		t.Fatal("curator not stamped")
	}
	if _, err = MutateAdminCuration(ctx, "verified", "", "POST", "staff", "duplicate", fields); err != ErrCurationConflict {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err = MutateAdminCuration(ctx, "verified", phone, "DELETE", "staff", "revoked", nil); err != nil {
		t.Fatal(err)
	}
	active, err := GetActiveVerifiedNumber(ctx, phone, time.Now())
	if err != nil || active != nil {
		t.Fatalf("revoked still active %v %v", active, err)
	}
	list, err := ListAdminCuration(ctx, "verified")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, v := range list {
		if v["phone_e164"] == phone {
			found = true
			if v["source_url"] != fields["source_url"] || v["status"] != "REVOKED" {
				t.Fatalf("revocation lost source %v", v)
			}
		}
	}
	if !found {
		t.Fatal("revocation disappeared")
	}
	emergency := model.EmergencyDirectory{Code: "TEST_NATIONAL", Scope: "NATIONAL", AgencyType: "POLISI", Name: "Public test office", Contacts: model.EmergencyContactDetails{ShortCode: "110"}, SourceURL: "https://example.test/contact", VerifiedBy: "staff", VerifiedAt: time.Now().Add(-time.Hour)}
	if _, err = MutateAdminCuration(ctx, "emergency", "", "POST", "staff", "verified", jsonMap(emergency)); err != nil {
		t.Fatal(err)
	}
	if _, err = MutateAdminCuration(ctx, "emergency", emergency.Code, "DELETE", "staff", "withdraw", nil); err != nil {
		t.Fatal(err)
	}
	audits, err := ListAdminAudit(ctx, 100)
	if err != nil || len(audits) < 4 {
		t.Fatalf("audit records %v %v", len(audits), err)
	}
}
func TestAdminCurationMemoryLifecycle(t *testing.T) {
	ResetMemoryForTest()
	defer ResetMemoryForTest()
	testCurationLifecycle(t)
}
func TestAdminExportExcludesPrivateFields(t *testing.T) {
	ResetMemoryForTest()
	defer ResetMemoryForTest()
	memMu.Lock()
	memModerationCases["private"] = model.ModerationCase{DisputeID: "CASE1", PhoneE164: "+628123450000", OwnerName: "secret-owner", Reason: "secret-reason", ContactRef: "secret-contact", SubmitterID: "secret-submitter", TrackingTokenHash: "secret-token", ResolutionNotes: "secret-notes"}
	memMu.Unlock()
	rows, err := ExportAdminDataset(context.Background(), "moderation_cases")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(rows)
	if strings.Contains(string(b), "secret-") {
		t.Fatal("PII escaped safe DTO")
	}
}
func TestAdminDatasetCuratorAuditAndIdempotency(t *testing.T) {
	ResetMemoryForTest()
	defer ResetMemoryForTest()
	now := time.Now().Add(-time.Hour)
	rows := []dataset.Record{{PhoneE164: "+622155500011", Kind: "BUSINESS", DisplayLabel: "Office", SourceURL: "https://example.test/source", VerifiedAt: now, ExpiresAt: now.Add(24 * time.Hour), Source: "TEST_CURATED"}}
	ctx := context.Background()
	r, e := ImportAdminDataset(ctx, rows, "verified", "admin-actor", "source checked", false)
	if e != nil || r.Written != 1 {
		t.Fatalf("import %+v %v", r, e)
	}
	v, e := GetActiveVerifiedNumber(ctx, rows[0].PhoneE164, time.Now())
	if e != nil || v == nil || v.VerifiedBy != "admin-actor" {
		t.Fatalf("curator %+v %v", v, e)
	}
	r, e = ImportAdminDataset(ctx, rows, "verified", "admin-actor", "repeat", false)
	if e != nil || r.Skipped != 1 {
		t.Fatalf("repeat %+v %v", r, e)
	}
}
func TestAdminCurationMongoLifecycleAndRecovery(t *testing.T) {
	uri := os.Getenv("CALLTAG_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("isolated Mongo required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	ResetMemoryForTest()
	MongoClient = client
	MongoDB = client.Database("stage6_curation_test")
	MongoConnected = true
	defer func() { _ = MongoDB.Drop(context.Background()); ResetMemoryForTest() }()
	testCurationLifecycle(t)
	// A failed sync leaves a durable pending receipt. Recovery completes it.
	_, err = MongoDB.Collection(ColSyncChanges).InsertOne(ctx, bson.M{"seq": int64(-1)})
	if err != nil {
		t.Fatal(err)
	}
	if err = MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColSyncChanges}, {Key: "validator", Value: bson.M{"seq": bson.M{"$lt": 0}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	fields := curationFixture()
	fields["display_label"] = "Updated office"
	if _, err = MutateAdminCuration(ctx, "verified", fields["phone_e164"].(string), "PATCH", "staff", "update", fields); err == nil {
		t.Fatal("sync failure expected")
	}
	count, _ := MongoDB.Collection(colCurationPending).CountDocuments(ctx, bson.M{})
	if count != 1 {
		t.Fatalf("pending receipt %d", count)
	}
	if err = MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColSyncChanges}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err = RecoverAdminCuration(ctx); err != nil {
		t.Fatal(err)
	}
	count, _ = MongoDB.Collection(colCurationPending).CountDocuments(ctx, bson.M{})
	if count != 0 {
		t.Fatal("receipt not cleared")
	}
	n, _ := MongoDB.Collection(ColSyncChanges).CountDocuments(ctx, bson.M{"kind": "EMERGENCY", "op": "DELETE", "key": "TEST_NATIONAL"})
	if n != 1 {
		t.Fatalf("emergency tombstone %d", n)
	}
}
