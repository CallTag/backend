package config

import (
	"context"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"testing"
	"time"
)

func TestVerifiedMongoSeedAndIdempotentImport(t *testing.T) {
	uri := os.Getenv("CALLTAG_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set CALLTAG_TEST_MONGO_URI to isolated MongoDB")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	ResetMemoryForTest()
	MongoClient = client
	MongoDB = client.Database("stage4b_verified")
	MongoConnected = true
	defer func() { _ = MongoDB.Drop(context.Background()); ResetMemoryForTest() }()
	if err := EnsureVerifiedIndexesAndSeed(ctx); err != nil {
		t.Fatal(err)
	}
	if err := EnsureVerifiedIndexesAndSeed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := MongoDB.Collection(ColVerifiedNumbers).InsertOne(ctx, bson.M{"phone_e164": "+622155500000"}); err == nil {
		t.Fatal("incomplete verified Mongo document accepted")
	}
	got, err := GetActiveVerifiedNumber(ctx, "1500888", time.Now().UTC())
	if err != nil || got == nil {
		t.Fatalf("seed %v %v", got, err)
	}
	n, err := MongoDB.Collection(ColSyncChanges).CountDocuments(ctx, bson.M{"kind": "VERIFIED_NUMBER", "key": "1500888"})
	if err != nil || n != 1 {
		t.Fatalf("seed sync %d %v", n, err)
	}
	now := time.Now().UTC()
	v := model.VerifiedNumber{PhoneE164: "+622155512345", Kind: "BUSINESS", DisplayLabel: "Test public business", SourceURL: "https://example.org/contact", VerifiedBy: "test", VerifiedAt: now, ExpiresAt: now.Add(time.Hour), Status: "ACTIVE"}
	if changed, err := UpsertVerifiedNumber(ctx, v); err != nil || !changed {
		t.Fatalf("insert %v %v", changed, err)
	}
	if changed, err := UpsertVerifiedNumber(ctx, v); err != nil || changed {
		t.Fatalf("repeat %v %v", changed, err)
	}
	// Kegagalan jurnal tidak boleh membuat impor ulang melewatkan sinkronisasi.
	if err := MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColSyncChanges}, {Key: "validator", Value: bson.M{"seq": bson.M{"$lt": 0}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	v.DisplayLabel = "Updated public business"
	if _, err := UpsertVerifiedNumber(ctx, v); err == nil {
		t.Fatal("expected failed sync write")
	}
	if err := MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColSyncChanges}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if changed, err := UpsertVerifiedNumber(ctx, v); err != nil || changed {
		t.Fatalf("repair sync: %v %v", changed, err)
	}
	n, err = MongoDB.Collection(ColSyncChanges).CountDocuments(ctx, bson.M{"kind": "VERIFIED_NUMBER", "key": v.PhoneE164})
	if err != nil || n != 2 {
		t.Fatalf("repaired sync count %d %v", n, err)
	}
	v.Status = "REVOKED"
	if _, err := UpsertVerifiedNumber(ctx, v); err != nil {
		t.Fatal(err)
	}
	if got, err := GetActiveVerifiedNumber(ctx, v.PhoneE164, time.Now().UTC()); err != nil || got != nil {
		t.Fatalf("revoked %v %v", got, err)
	}
	if err := EnsureVerifiedIndexesAndSeed(ctx); err != nil {
		t.Fatal(err)
	}
}
