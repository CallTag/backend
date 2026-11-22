package config

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/CallTag/backend/model"
	"github.com/CallTag/backend/seeds"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ColVerifiedNumbers = "verified_numbers"

var memVerifiedNumbers = make(map[string]model.VerifiedNumber)
var verifiedPhonePattern = regexp.MustCompile(`^(\+[1-9][0-9]{6,14}|[1-9][0-9]{2,5}|150[0-9]{4})$`)

func validateVerifiedNumber(v model.VerifiedNumber) error {
	u, err := url.Parse(v.SourceURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || !verifiedPhonePattern.MatchString(v.PhoneE164) || strings.TrimSpace(v.DisplayLabel) == "" || strings.TrimSpace(v.VerifiedBy) == "" || v.VerifiedAt.IsZero() || v.ExpiresAt.IsZero() || !v.ExpiresAt.After(v.VerifiedAt) {
		return errors.New("verified number requires canonical phone, HTTPS source, curator, label and valid verification/expiry dates")
	}
	if v.Kind != "OFFICIAL" && v.Kind != "BUSINESS" && v.Kind != "PUBLIC_SERVICE" {
		return errors.New("invalid verified kind")
	}
	if v.Status != "ACTIVE" && v.Status != "REVOKED" && v.Status != "INACTIVE" {
		return errors.New("invalid verified status")
	}
	return nil
}

// GetActiveVerifiedNumber memberi verifikasi hanya untuk bukti aktif yang belum kedaluwarsa.
func GetActiveVerifiedNumber(ctx context.Context, phone string, now time.Time) (*model.VerifiedNumber, error) {
	var item model.VerifiedNumber
	if MongoConnected && MongoDB != nil {
		err := MongoDB.Collection(ColVerifiedNumbers).FindOne(ctx, bson.M{"phone_e164": phone, "status": "ACTIVE", "expires_at": bson.M{"$gt": now}}).Decode(&item)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	} else {
		memMu.RLock()
		item = memVerifiedNumbers[phone]
		memMu.RUnlock()
	}
	if item.Status != "ACTIVE" || !item.ExpiresAt.After(now) || item.VerifiedAt.After(now) || validateVerifiedNumber(item) != nil {
		return nil, nil
	}
	return &item, nil
}

func verifiedPayload(v model.VerifiedNumber) map[string]interface{} {
	return map[string]interface{}{"phone_e164": v.PhoneE164, "kind": v.Kind, "display_label": v.DisplayLabel, "source_url": v.SourceURL, "verified_by": v.VerifiedBy, "verified_at": v.VerifiedAt, "expires_at": v.ExpiresAt, "status": v.Status}
}

// UpsertVerifiedNumber mempertahankan ID dan mencatat perubahan material untuk sinkronisasi.
func UpsertVerifiedNumber(ctx context.Context, item model.VerifiedNumber) (bool, error) {
	item.VerifiedAt = item.VerifiedAt.UTC().Truncate(time.Millisecond)
	item.ExpiresAt = item.ExpiresAt.UTC().Truncate(time.Millisecond)
	if err := validateVerifiedNumber(item); err != nil {
		return false, err
	}
	if MongoConnected && MongoDB != nil {
		col := MongoDB.Collection(ColVerifiedNumbers)
		// Impor identik tidak membuat perubahan sinkronisasi baru.
		filter := bson.M{"phone_e164": item.PhoneE164}
		var stored struct {
			model.VerifiedNumber `bson:",inline"`
			SyncPending          bool `bson:"sync_pending"`
		}
		err := col.FindOne(ctx, filter).Decode(&stored)
		prev := stored.VerifiedNumber
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return false, err
		}
		item.ID = prev.ID
		if reflect.DeepEqual(verifiedPayload(prev), verifiedPayload(item)) {
			if stored.SyncPending {
				return false, flushVerifiedSync(ctx, item)
			}
			return false, nil
		}
		payload := verifiedPayload(item)
		payload["sync_pending"] = true
		_, err = col.UpdateOne(ctx, filter, bson.M{"$set": payload}, options.Update().SetUpsert(true))
		if err != nil {
			return false, err
		}
		return true, flushVerifiedSync(ctx, item)
	}
	memMu.Lock()
	defer memMu.Unlock()
	prev, ok := memVerifiedNumbers[item.PhoneE164]
	if ok && reflect.DeepEqual(verifiedPayload(prev), verifiedPayload(item)) {
		return false, nil
	}
	item.ID = prev.ID
	if item.ID.IsZero() {
		item.ID = primitive.NewObjectID()
	}
	memVerifiedNumbers[item.PhoneE164] = item
	appendSyncChangeMemoryLocked("UPSERT", "VERIFIED_NUMBER", item.PhoneE164, verifiedPayload(item))
	return true, nil
}

func initVerifiedMemorySeedLocked() {
	items, err := seeds.LoadVerifiedSeeds()
	if err != nil {
		return
	}
	for _, item := range items {
		if validateVerifiedNumber(item) == nil {
			memVerifiedNumbers[item.PhoneE164] = item
		}
	}
}

// EnsureVerifiedIndexesAndSeed mempertahankan hasil kurasi lama, termasuk pencabutan verifikasi.
func EnsureVerifiedIndexesAndSeed(ctx context.Context) error {
	col := MongoDB.Collection(ColVerifiedNumbers)
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "phone_e164", Value: 1}}, Options: options.Index().SetUnique(true).SetName("idx_verified_phone")}); err != nil {
		return err
	}
	// Validator moderate mengizinkan migrasi dokumen lama sambil menolak entri baru yang tidak lengkap.
	properties := bson.M{}
	for _, key := range []string{"phone_e164", "display_label", "source_url", "verified_by"} {
		properties[key] = bson.M{"bsonType": "string", "minLength": 1}
	}
	properties["kind"] = bson.M{"enum": bson.A{"OFFICIAL", "BUSINESS", "PUBLIC_SERVICE"}}
	properties["status"] = bson.M{"enum": bson.A{"ACTIVE", "REVOKED", "INACTIVE"}}
	properties["verified_at"] = bson.M{"bsonType": "date"}
	properties["expires_at"] = bson.M{"bsonType": "date"}
	validator := bson.M{"$jsonSchema": bson.M{"bsonType": "object", "required": bson.A{"phone_e164", "kind", "display_label", "source_url", "verified_by", "verified_at", "expires_at", "status"}, "properties": properties}}
	if err := MongoDB.RunCommand(ctx, bson.D{{Key: "collMod", Value: ColVerifiedNumbers}, {Key: "validator", Value: validator}, {Key: "validationLevel", Value: "moderate"}}).Err(); err != nil {
		return err
	}
	items, err := seeds.LoadVerifiedSeeds()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := validateVerifiedNumber(item); err != nil {
			return err
		}
		payload := verifiedPayload(item)
		payload["sync_pending"] = true
		if _, err := col.UpdateOne(ctx, bson.M{"phone_e164": item.PhoneE164}, bson.M{"$setOnInsert": payload}, options.Update().SetUpsert(true)); err != nil {
			return err
		}
		var pending model.VerifiedNumber
		err := col.FindOne(ctx, bson.M{"phone_e164": item.PhoneE164, "sync_pending": true}).Decode(&pending)
		if err == nil {
			if err := flushVerifiedSync(ctx, pending); err != nil {
				return err
			}
		} else if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
	}
	return nil
}

// flushVerifiedSync memulihkan jurnal yang tertunda saat upsert sebelumnya gagal.
func flushVerifiedSync(ctx context.Context, item model.VerifiedNumber) error {
	if _, err := appendSyncChangeMongo(ctx, "UPSERT", "VERIFIED_NUMBER", item.PhoneE164, verifiedPayload(item)); err != nil {
		return err
	}
	_, err := MongoDB.Collection(ColVerifiedNumbers).UpdateOne(ctx, bson.M{"phone_e164": item.PhoneE164, "verified_at": item.VerifiedAt, "expires_at": item.ExpiresAt, "status": item.Status}, bson.M{"$unset": bson.M{"sync_pending": ""}})
	return err
}
