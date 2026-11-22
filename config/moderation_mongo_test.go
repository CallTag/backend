package config

import (
	"context"
	"github.com/CallTag/backend/internal/dataset"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"testing"
	"time"
)

func TestModerationMongoRecoveryAfterSyncFailure(t *testing.T) {
	uri := os.Getenv("CALLTAG_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set CALLTAG_TEST_MONGO_URI to isolated MongoDB")
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
	MongoDB = client.Database("stage6_moderation_" + primitive.NewObjectID().Hex())
	MongoConnected = true
	defer func() { _ = MongoDB.Drop(context.Background()); ResetMemoryForTest() }()
	if err = EnsureIndexesAndSeed(ctx); err != nil {
		t.Fatal(err)
	}
	if err = EnsureAuditIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	phone := "+6287773334455"
	now := time.Now().UTC()
	for i := 0; i < 12; i++ {
		_, err = MongoDB.Collection(ColSpamReports).InsertOne(ctx, model.SpamReport{ID: primitive.NewObjectID(), PhoneE164: phone, ReporterUserID: primitive.NewObjectID().Hex(), CategoryCode: model.CategoryAPKSniffing, Status: "VALIDATED", CreatedAt: now.Add(-time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = recomputePhoneTagMongo(ctx, MongoDB.Collection(ColSpamReports), MongoDB.Collection(ColPhoneTags), phone); err != nil {
		t.Fatal(err)
	}
	c, _, err := CreateDisputeCase(ctx, model.DisputeRequest{Phone: phone, OwnerName: "Owner", Reason: "Incorrect"}, "test:owner")
	if err != nil {
		t.Fatal(err)
	}
	if err = MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColSyncChanges}, {Key: "validator", Value: bson.M{"seq": bson.M{"$lt": 0}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err = DecideAdminCase(ctx, c.DisputeID, 1, "ACCEPT", "Evidence checked", "staff"); err == nil {
		t.Fatal("expected sync failure")
	}
	reserved, err := adminCase(ctx, c.DisputeID)
	if err != nil || reserved.Status != "PROCESSING" || reserved.Decision.Done {
		t.Fatalf("missing recovery intent %+v %v", reserved, err)
	}
	if err = MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColSyncChanges}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err = RecoverModerationDecisions(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = DecideAdminCase(ctx, c.DisputeID, 1, "ACCEPT", "Evidence checked", "staff"); err != nil {
		t.Fatal("retry", err)
	}
	p, reps, err := GetAdminPhone(ctx, phone)
	if err != nil || p.TotalReports != 0 || len(reps) != 12 {
		t.Fatalf("correction %+v reports=%d err=%v", p, len(reps), err)
	}
	n, err := MongoDB.Collection(ColAuditLogs).CountDocuments(ctx, bson.M{"target": c.DisputeID})
	if err != nil || n != 1 {
		t.Fatalf("audit=%d %v", n, err)
	}
	n, err = MongoDB.Collection(ColSyncChanges).CountDocuments(ctx, bson.M{"kind": "PHONE_TAG", "key": phone, "op": "DELETE"})
	if err != nil || n < 1 {
		t.Fatalf("revocation=%d %v", n, err)
	}
}

func TestEnsureIndexesAndSeedRecoversCrashedAdminOperations(t *testing.T) {
	uri := os.Getenv("CALLTAG_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set CALLTAG_TEST_MONGO_URI to isolated MongoDB")
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
	MongoDB = client.Database("stage6_startup_recovery_" + primitive.NewObjectID().Hex())
	MongoConnected = true
	defer func() { _ = MongoDB.Drop(context.Background()); ResetMemoryForTest() }()

	// Simulasikan kondisi crash sebelum server restart:
	// 1) Keputusan moderasi tertahan PROCESSING (admin_decision.done=false)
	caseID := "DSP_" + primitive.NewObjectID().Hex()
	now := time.Now().UTC().Truncate(time.Millisecond)
	pendingCase := model.ModerationCase{
		ID:          primitive.NewObjectID(),
		DisputeID:   caseID,
		Type:        "DISPUTE",
		PhoneE164:   "+6287774445566",
		OwnerName:   "Pemilik Terdampak",
		Reason:      "Nomor usaha sah",
		SubmitterID: "guest:recovery",
		Status:      "PROCESSING",
		Version:     2,
		SubmittedAt: now.Add(-time.Hour),
		UpdatedAt:   now,
		Decision: &model.AdminDecision{
			Actor:      "staff:recovery",
			Action:     "ACCEPT",
			Reason:     "Bukti sah diverifikasi",
			Version:    1,
			BeforeHash: "abc",
			At:         now,
			Done:       false,
		},
	}
	if _, err = MongoDB.Collection(ColModerationCases).InsertOne(ctx, pendingCase); err != nil {
		t.Fatal(err)
	}

	// 2) Impor admin terhenti sebelum receipt dihapus
	importPhone := "+62215559988"
	if _, err = MongoDB.Collection(colAdminImportPending).InsertOne(ctx, adminImportPending{
		ID:     primitive.NewObjectID(),
		Actor:  "staff:recovery",
		Reason: "Impor darurat terhenti",
		Kind:   "verified",
		Rows: []dataset.Record{{
			PhoneE164:    importPhone,
			Kind:         "BUSINESS",
			DisplayLabel: "Bisnis Recovery",
			SourceURL:    "https://example.test/recovery",
			VerifiedAt:   now.Add(-time.Hour),
			ExpiresAt:    now.Add(24 * time.Hour),
			Source:       "KURASI_STAF",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	// 3) Kurasi admin terhenti sebelum receipt dihapus
	curCode := "RECOVERY_LOCAL_01"
	em := jsonMap(model.EmergencyDirectory{
		ID:          primitive.NewObjectID(),
		Code:        curCode,
		Scope:       "NATIONAL",
		AgencyType:  "POLISI",
		Name:        "Posko Recovery",
		Contacts:    model.EmergencyContactDetails{ShortCode: "110"},
		SourceURL:   "https://example.test/emergency",
		VerifiedBy:  "staff:recovery",
		VerifiedAt:  now.Add(-time.Hour),
	})
	if _, err = MongoDB.Collection(colCurationPending).InsertOne(ctx, curationPending{
		ID:     primitive.NewObjectID(),
		Kind:   "emergency",
		Key:    curCode,
		Actor:  "staff:recovery",
		Reason: "Kurasi terhenti",
		Action: "POST",
		After:  em,
	}); err != nil {
		t.Fatal(err)
	}

	// Simulasi startup ulang server
	if err = EnsureIndexesAndSeed(ctx); err != nil {
		t.Fatal(err)
	}

	recoveredCase, err := adminCase(ctx, caseID)
	if err != nil || recoveredCase.Status != "ACCEPTED" || recoveredCase.Decision == nil || !recoveredCase.Decision.Done {
		t.Fatalf("keputusan moderasi belum pulih saat startup: %+v err=%v", recoveredCase, err)
	}
	v, err := GetActiveVerifiedNumber(ctx, importPhone, time.Now().UTC())
	if err != nil || v == nil || v.DisplayLabel != "Bisnis Recovery" {
		t.Fatalf("impor tertunda belum pulih saat startup: %+v err=%v", v, err)
	}
	if n, _ := MongoDB.Collection(colAdminImportPending).CountDocuments(ctx, bson.M{}); n != 0 {
		t.Fatalf("receipt impor masih tersisa: %d", n)
	}
	if n, _ := MongoDB.Collection(colCurationPending).CountDocuments(ctx, bson.M{}); n != 0 {
		t.Fatalf("receipt kurasi masih tersisa: %d", n)
	}
	if n, _ := MongoDB.Collection(ColEmergencyDirectory).CountDocuments(ctx, bson.M{"code": curCode}); n != 1 {
		t.Fatalf("entri kurasi belum tersimpan saat startup: %d", n)
	}
}
