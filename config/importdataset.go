package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/internal/dataset"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ImportResult struct {
	Validated int  `json:"validated"`
	Written   int  `json:"written"`
	Skipped   int  `json:"skipped"`
	DryRun    bool `json:"dry_run"`
}

// ImportDataset validates every row before the first write. External reports keep
// their provenance and never grant an official blacklist or verified badge.
func ImportDataset(ctx context.Context, rows []dataset.Record, kind string, dryRun bool) (ImportResult, error) {
	result := ImportResult{DryRun: dryRun}
	data, err := json.Marshal(rows)
	if err != nil {
		return result, err
	}
	rows, err = dataset.Parse(bytes.NewReader(data), ".json", kind, "")
	if err != nil {
		return result, err
	}
	result.Validated = len(rows)
	if dryRun {
		return result, nil
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var changed bool
		if kind == "verified" {
			changed, err = UpsertVerifiedNumber(ctx, model.VerifiedNumber{PhoneE164: row.PhoneE164, Kind: row.Kind, DisplayLabel: row.DisplayLabel, SourceURL: row.SourceURL, VerifiedBy: "dataset:" + row.Source, VerifiedAt: row.VerifiedAt, ExpiresAt: row.ExpiresAt, Status: "ACTIVE"})
		} else {
			changed, err = importDatasetReport(ctx, row)
		}
		if err != nil {
			return result, err
		}
		if changed {
			result.Written++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}

func importDatasetReport(ctx context.Context, row dataset.Record) (bool, error) {
	data, _ := json.Marshal(row)
	sum := sha256.Sum256(data)
	key := "dataset:" + hex.EncodeToString(sum[:])
	now := time.Now().UTC()
	report := model.SpamReport{ID: primitive.NewObjectID(), PhoneE164: row.PhoneE164, Source: row.Source, ReporterUserID: "dataset:" + row.Source + ":" + row.PhoneE164, IdempotencyKey: key, CategoryCode: row.CategoryCode, CustomTag: row.CustomTag, Comment: row.Comment, CallType: "DATASET", ReporterTrustSnapshot: 1, Status: "IMPORTED", CreatedAt: now}
	if MongoConnected && MongoDB != nil {
		col := MongoDB.Collection(ColSpamReports)
		encoded, _ := bson.Marshal(report)
		var document bson.M
		if err := bson.Unmarshal(encoded, &document); err != nil {
			return false, err
		}
		document["dataset_sync_pending"] = true
		filter := bson.M{"reporter_user_id": report.ReporterUserID, "idempotency_key": key}
		result, err := col.UpdateOne(ctx, filter, bson.M{"$setOnInsert": document}, options.Update().SetUpsert(true))
		if err != nil {
			return false, err
		}
		if result.UpsertedCount == 0 {
			var pending struct {
				Pending bool `bson:"dataset_sync_pending"`
			}
			if err := col.FindOne(ctx, filter).Decode(&pending); err != nil {
				return false, err
			}
			if !pending.Pending {
				return false, nil
			}
		}
		_, err = recomputePhoneTagMongo(ctx, col, MongoDB.Collection(ColPhoneTags), row.PhoneE164)
		if err != nil {
			return false, err
		}
		_, err = col.UpdateOne(ctx, filter, bson.M{"$unset": bson.M{"dataset_sync_pending": ""}})
		return true, err
	}
	if IsProdEnv() {
		return false, errors.New("MongoDB is required for production imports")
	}
	memMu.Lock()
	defer memMu.Unlock()
	for _, r := range memSpamReports[row.PhoneE164] {
		if r.IdempotencyKey == key && r.ReporterUserID == report.ReporterUserID {
			return false, nil
		}
	}
	reports := append(memSpamReports[row.PhoneE164], report)
	memSpamReports[row.PhoneE164] = reports
	var prev *model.PhoneTag
	if item, ok := memPhoneTags[row.PhoneE164]; ok {
		prev = &item
	}
	memSyncCounter++
	aggregated := helper.AggregatePhoneTag(prev, reports, memSyncCounter)
	if aggregated.ID.IsZero() {
		aggregated.ID = primitive.NewObjectID()
	}
	memPhoneTags[row.PhoneE164] = aggregated
	appendSyncChangeMemoryLocked("UPSERT", "PHONE_TAG", row.PhoneE164, buildPhoneTagSyncPayload(aggregated))
	if aggregated.PendingDangerReview {
		ensurePendingDangerReviewCaseMemLocked(row.PhoneE164, now)
	}
	return true, nil
}
