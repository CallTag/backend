package config

import (
	"context"
	"github.com/CallTag/backend/internal/dataset"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"testing"
)

func TestImportDatasetIdempotenceAndProvenance(t *testing.T) {
	ResetMemoryForTest()
	checkDatasetImport(t)
}

func checkDatasetImport(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	before, err := GetSyncChanges(ctx, "", 500)
	if err != nil {
		t.Fatal(err)
	}
	rows := []dataset.Record{{PhoneE164: "+12025550199", CategoryCode: model.CategoryTelemarketing, CustomTag: "External open list", Source: "EXTERNAL_OPEN_LIST"}}
	result, err := ImportDataset(ctx, rows, "scam", true)
	if err != nil || result.Written != 0 {
		t.Fatalf("dry run: %+v %v", result, err)
	}
	if pt, _, _ := GetPhoneLookup(ctx, rows[0].PhoneE164); pt != nil {
		t.Fatal("dry run wrote data")
	}
	result, err = ImportDataset(ctx, rows, "scam", false)
	if err != nil || result.Written != 1 {
		t.Fatalf("first import: %+v %v", result, err)
	}
	result, err = ImportDataset(ctx, rows, "scam", false)
	if err != nil || result.Skipped != 1 {
		t.Fatalf("replay: %+v %v", result, err)
	}
	pt, reports, err := GetPhoneLookup(ctx, rows[0].PhoneE164)
	if err != nil || pt == nil || pt.IsOfficialBlacklisted || len(reports) != 1 || reports[0].Source != "EXTERNAL_OPEN_LIST" || reports[0].ReporterUserID != "" {
		t.Fatalf("provenance/blacklist: %+v %+v %v", pt, reports, err)
	}
	after, err := GetSyncChanges(ctx, before.NextCursor, 500)
	if err != nil || len(after.Items) != 1 || after.Items[0].Kind != "PHONE_TAG" {
		t.Fatalf("journal: %+v %v", after, err)
	}
	neutral := []dataset.Record{{PhoneE164: "+12025550197", CategoryCode: model.CategoryCode("INFO_TAG"), CustomTag: "Kurir Paket", Source: "KURASI_TIM"}}
	result, err = ImportDataset(ctx, neutral, "tag_context", false)
	if err != nil || result.Written != 1 {
		t.Fatalf("neutral import: %+v %v", result, err)
	}
	pt, reports, err = GetPhoneLookup(ctx, neutral[0].PhoneE164)
	if err != nil || pt == nil || pt.RiskScore != 0 || pt.IsDanger || len(pt.Tags) != 1 || len(reports) != 1 || reports[0].Source != "KURASI_TIM" {
		t.Fatalf("neutral persisted: %+v %+v %v", pt, reports, err)
	}
	result, err = ImportDataset(ctx, neutral, "tag_context", false)
	if err != nil || result.Skipped != 1 {
		t.Fatalf("neutral replay: %+v %v", result, err)
	}

}

func TestImportDatasetMongo(t *testing.T) {
	uri := os.Getenv("CALLTAG_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set CALLTAG_TEST_MONGO_URI for isolated Mongo test")
	}
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(ctx)
	ResetMemoryForTest()
	MongoClient = client
	MongoDB = client.Database("stage4b_import_test")
	MongoConnected = true
	defer func() { MongoDB.Drop(ctx); ResetMemoryForTest() }()
	if err := MongoDB.Drop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := EnsureIndexesAndSeed(ctx); err != nil {
		t.Fatal(err)
	}
	checkDatasetImport(t)
}

func TestImportDatasetRepairsFailedMongoAggregation(t *testing.T) {
	uri := os.Getenv("CALLTAG_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set CALLTAG_TEST_MONGO_URI")
	}
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(ctx)
	ResetMemoryForTest()
	MongoClient = client
	MongoDB = client.Database("stage4b_import_recovery_test")
	MongoConnected = true
	defer func() { MongoDB.Drop(ctx); ResetMemoryForTest() }()
	MongoDB.Drop(ctx)
	if err := MongoDB.CreateCollection(ctx, ColPhoneTags, options.CreateCollection().SetValidator(map[string]interface{}{"required_unavailable_field": true})); err != nil {
		t.Fatal(err)
	}
	rows := []dataset.Record{{PhoneE164: "+12025550198", CategoryCode: model.CategoryTelemarketing, CustomTag: "External", Source: "EXTERNAL_OPEN_LIST"}}
	if _, err := ImportDataset(ctx, rows, "scam", false); err == nil {
		t.Fatal("expected phone tag validation failure")
	}
	if err := MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColPhoneTags}, {Key: "validator", Value: bson.M{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	result, err := ImportDataset(ctx, rows, "scam", false)
	if err != nil || result.Written != 1 {
		t.Fatalf("recovery: %+v %v", result, err)
	}
	result, err = ImportDataset(ctx, rows, "scam", false)
	if err != nil || result.Skipped != 1 {
		t.Fatalf("replay: %+v %v", result, err)
	}
	journal, err := GetSyncChanges(ctx, "", 100)
	if err != nil || len(journal.Items) != 1 {
		t.Fatalf("journal: %+v %v", journal, err)
	}
}

func TestImportDatasetValidatesWholeBatchAndImportsMITFixture(t *testing.T) {
	ResetMemoryForTest()
	ctx := context.Background()
	f, err := os.Open("../internal/dataset/testdata/callshield_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := dataset.Parse(f, ".json", "scam", "EXTERNAL_OPEN_LIST")
	if err != nil {
		t.Fatal(err)
	}
	bad := append(append([]dataset.Record{}, rows...), dataset.Record{PhoneE164: "bad"})
	if _, err := ImportDataset(ctx, bad, "scam", false); err == nil {
		t.Fatal("accepted invalid trailing row")
	}
	if pt, _, _ := GetPhoneLookup(ctx, rows[0].PhoneE164); pt != nil {
		t.Fatal("invalid batch wrote earlier rows")
	}
	first, err := ImportDataset(ctx, rows, "scam", false)
	if err != nil || first.Written != 2 {
		t.Fatalf("fixture import: %+v %v", first, err)
	}
	second, err := ImportDataset(ctx, rows, "scam", false)
	if err != nil || second.Skipped != 2 {
		t.Fatalf("fixture replay: %+v %v", second, err)
	}
}
