package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CallTag/backend/internal/dataset"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

var curationMu sync.Mutex
var ErrCurationNotFound = errors.New("curated entry not found")
var ErrCurationConflict = errors.New("curated entry already exists")

const colCurationPending = "admin_curation_pending"

type curationPending struct {
	ID     primitive.ObjectID     `bson:"_id"`
	Kind   string                 `bson:"kind"`
	Key    string                 `bson:"key"`
	Actor  string                 `bson:"actor"`
	Reason string                 `bson:"reason"`
	Action string                 `bson:"action"`
	Before map[string]interface{} `bson:"before"`
	After  map[string]interface{} `bson:"after"`
}

func curationCollection(kind string) (string, string, string, error) {
	switch kind {
	case "verified", "verified-numbers":
		return ColVerifiedNumbers, "phone_e164", "VERIFIED_NUMBER", nil
	case "emergency":
		return ColEmergencyDirectory, "code", "EMERGENCY", nil
	case "channels", "official-channels":
		return ColOfficialChannels, "channel_slug", "OFFICIAL_CHANNEL", nil
	}
	return "", "", "", errors.New("unsupported curated dataset")
}
func jsonMap(v interface{}) map[string]interface{} {
	b, _ := json.Marshal(v)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	return m
}

// ListAdminCuration includes inactive entries so revocations remain inspectable.
func ListAdminCuration(ctx context.Context, kind string) ([]map[string]interface{}, error) {
	col, _, _, err := curationCollection(kind)
	if err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	if MongoConnected && MongoDB != nil {
		cur, err := MongoDB.Collection(col).Find(ctx, bson.M{}, options.Find().SetLimit(10000))
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		switch col {
		case ColVerifiedNumbers:
			var rows []model.VerifiedNumber
			err = cur.All(ctx, &rows)
			for _, v := range rows {
				out = append(out, jsonMap(v))
			}
		case ColEmergencyDirectory:
			var rows []model.EmergencyDirectory
			err = cur.All(ctx, &rows)
			for _, v := range rows {
				out = append(out, jsonMap(v))
			}
		case ColOfficialChannels:
			var rows []model.OfficialChannel
			err = cur.All(ctx, &rows)
			for _, v := range rows {
				out = append(out, jsonMap(v))
			}
		}
		return out, err
	}
	if IsProdEnv() {
		return nil, errors.New("MongoDB required")
	}
	memMu.RLock()
	defer memMu.RUnlock()
	switch col {
	case ColVerifiedNumbers:
		for _, v := range memVerifiedNumbers {
			out = append(out, jsonMap(v))
		}
	case ColEmergencyDirectory:
		for _, v := range memEmergency {
			out = append(out, jsonMap(v))
		}
	case ColOfficialChannels:
		for _, v := range memOfficialChannels {
			out = append(out, jsonMap(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["id"]) < fmt.Sprint(out[j]["id"]) })
	return out, nil
}
func sourceValid(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

// ValidateEmergencyCuration is also used by batch imports before any row is written.
func ValidateEmergencyCuration(v model.EmergencyDirectory) error {
	if !sourceValid(v.SourceURL) || strings.TrimSpace(v.VerifiedBy) == "" || v.VerifiedAt.IsZero() || v.VerifiedAt.After(time.Now().Add(time.Minute)) || v.Code == "" || v.Name == "" || (v.Scope != "NATIONAL" && v.Scope != "LOCAL") {
		return errors.New("emergency requires code, name, scope, HTTPS source, curator and valid verified_at")
	}
	switch v.AgencyType {
	case "TERPADU", "POLISI", "DAMKAR", "AMBULANS_RS", "SAR", "PLN", "JASA_MARGA":
	default:
		return errors.New("invalid emergency agency_type")
	}
	if v.Scope == "LOCAL" && (v.Province == "" || v.CityRegency == "") {
		return errors.New("local emergency requires province and city_regency")
	}
	if v.Contacts.ShortCode == "" && len(v.Contacts.PhoneNumbers) == 0 && v.Contacts.WhatsAppNumber == "" {
		return errors.New("emergency requires a contact")
	}
	for _, p := range append(append([]string{}, v.Contacts.PhoneNumbers...), v.Contacts.ShortCode, v.Contacts.WhatsAppNumber) {
		if p != "" && !verifiedPhonePattern.MatchString(p) {
			return errors.New("invalid emergency contact phone")
		}
	}
	if v.Location != nil && (v.Location.Type != "Point" || len(v.Location.Coordinates) != 2 || v.Location.Coordinates[0] < -180 || v.Location.Coordinates[0] > 180 || v.Location.Coordinates[1] < -90 || v.Location.Coordinates[1] > 90) {
		return errors.New("invalid GeoJSON location")
	}
	return nil
}
func validateCuration(kind string, m map[string]interface{}, actor string) (map[string]interface{}, error) {
	b, _ := json.Marshal(m)
	switch kind {
	case "verified", "verified-numbers":
		var v model.VerifiedNumber
		if e := json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		v.VerifiedBy = actor
		if v.Status == "" {
			v.Status = "ACTIVE"
		}
		if e := validateVerifiedNumber(v); e != nil {
			return nil, e
		}
		if v.VerifiedAt.After(time.Now().Add(time.Minute)) {
			return nil, errors.New("verified_at is in the future")
		}
		return jsonMap(v), nil
	case "emergency":
		var v model.EmergencyDirectory
		if e := json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		v.VerifiedBy = actor
		if e := ValidateEmergencyCuration(v); e != nil {
			return nil, e
		}
		return jsonMap(v), nil
	default:
		var v model.OfficialChannel
		if e := json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		v.VerifiedBy = actor
		v.UpdatedAt = time.Now().UTC()
		if !sourceValid(v.SourceURL) || v.VerifiedAt.IsZero() || v.VerifiedAt.After(time.Now().Add(time.Minute)) || v.ChannelSlug == "" || v.Title == "" || v.AuthorityName == "" || !sourceValid(v.ActionLinks.WebsiteURL) {
			return nil, errors.New("channel requires slug, title, authority, HTTPS website/source and valid verified_at")
		}
		if v.ActionLinks.WhatsAppURL != "" && !sourceValid(v.ActionLinks.WhatsAppURL) {
			return nil, errors.New("invalid WhatsApp URL")
		}
		return jsonMap(v), nil
	}
}

// MutateAdminCuration persists a recovery record before changing curated data.
// The record remains until both append-only audit and sync journal are durable.
func MutateAdminCuration(ctx context.Context, kind, key, method, actor, reason string, fields map[string]interface{}) (map[string]interface{}, error) {
	curationMu.Lock()
	defer curationMu.Unlock()
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return nil, errors.New("actor and reason (maximum 2000 characters) required")
	}
	_, keyField, _, err := curationCollection(kind)
	if err != nil {
		return nil, err
	}
	if err = RecoverAdminCuration(ctx); err != nil {
		return nil, err
	}
	rows, err := ListAdminCuration(ctx, kind)
	if err != nil {
		return nil, err
	}
	var before map[string]interface{}
	if method == "POST" {
		key, _ = fields[keyField].(string)
	}
	for _, row := range rows {
		if fmt.Sprint(row[keyField]) == key || fmt.Sprint(row["id"]) == key {
			before = row
			key = fmt.Sprint(row[keyField])
			break
		}
	}
	if method == "POST" && before != nil {
		return nil, ErrCurationConflict
	}
	if method != "POST" && method != "PUT" && before == nil {
		return nil, ErrCurationNotFound
	}
	after := map[string]interface{}{}
	if method == "PUT" && before == nil {
		after[keyField] = key
	}
	if before != nil {
		for k, v := range before {
			after[k] = v
		}
	}
	if method == "DELETE" {
		switch kind {
		case "verified", "verified-numbers":
			after["status"] = "REVOKED"
		case "emergency":
			after = nil
		default:
			after["is_active"] = false
		}
	} else {
		for k, v := range fields {
			if k != "reason" && k != "id" && k != "sync_version" && k != "verified_by" {
				after[k] = v
			}
		}
		if before != nil && fmt.Sprint(after[keyField]) != key {
			return nil, errors.New("identity field is immutable")
		}
		if before == nil {
			after["id"] = primitive.NewObjectID().Hex()
		}
		after, err = validateCuration(kind, after, actor)
		if err != nil {
			return nil, err
		}
	}
	p := curationPending{ID: primitive.NewObjectID(), Kind: kind, Key: key, Actor: actor, Reason: reason, Action: method, Before: before, After: after}
	if MongoConnected && MongoDB != nil {
		if _, err = MongoDB.Collection(colCurationPending).InsertOne(ctx, p); err != nil {
			return nil, err
		}
	}
	if err = finishCuration(ctx, p); err != nil {
		return nil, err
	}
	return after, nil
}
func RecoverAdminCuration(ctx context.Context) error {
	if !MongoConnected || MongoDB == nil {
		return nil
	}
	cur, err := MongoDB.Collection(colCurationPending).Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	var rows []curationPending
	if err = cur.All(ctx, &rows); err != nil {
		return err
	}
	for _, p := range rows {
		if err = finishCuration(ctx, p); err != nil {
			return err
		}
	}
	return nil
}
func recoverAdminStartup(ctx context.Context) error {
	curationMu.Lock()
	defer curationMu.Unlock()
	if err := RecoverAdminCuration(ctx); err != nil {
		return err
	}
	return RecoverAdminImports(ctx)
}

func finishCuration(ctx context.Context, p curationPending) error {
	col, keyField, syncKind, _ := curationCollection(p.Kind)
	if col == ColVerifiedNumbers {
		var v model.VerifiedNumber
		b, _ := json.Marshal(p.After)
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		if _, err := UpsertVerifiedNumber(ctx, v); err != nil {
			return err
		}
	} else if MongoConnected && MongoDB != nil {
		filter := bson.M{keyField: p.Key}
		if p.After == nil {
			if _, err := MongoDB.Collection(col).DeleteOne(ctx, filter); err != nil {
				return err
			}
		} else {
			b, _ := json.Marshal(p.After)
			var v interface{}
			if col == ColEmergencyDirectory {
				var row model.EmergencyDirectory
				_ = json.Unmarshal(b, &row)
				v = row
			} else {
				var row model.OfficialChannel
				_ = json.Unmarshal(b, &row)
				v = row
			}
			if _, err := MongoDB.Collection(col).ReplaceOne(ctx, filter, v, options.Replace().SetUpsert(true)); err != nil {
				return err
			}
		}
	} else {
		memMu.Lock()
		if col == ColEmergencyDirectory {
			next := []model.EmergencyDirectory{}
			for _, v := range memEmergency {
				if v.Code != p.Key {
					next = append(next, v)
				}
			}
			if p.After != nil {
				var v model.EmergencyDirectory
				b, _ := json.Marshal(p.After)
				_ = json.Unmarshal(b, &v)
				next = append(next, v)
			}
			memEmergency = next
		} else {
			next := []model.OfficialChannel{}
			for _, v := range memOfficialChannels {
				if v.ChannelSlug != p.Key {
					next = append(next, v)
				}
			}
			if p.After != nil {
				var v model.OfficialChannel
				b, _ := json.Marshal(p.After)
				_ = json.Unmarshal(b, &v)
				next = append(next, v)
			}
			memOfficialChannels = next
		}
		memMu.Unlock()
	}
	if err := RecordAdminAudit(ctx, p.Actor, "CURATION_"+p.Action, p.Kind+":"+p.Key, p.Reason, p.Before, p.After); err != nil {
		return err
	}
	if col != ColVerifiedNumbers {
		op := "UPSERT"
		if p.After == nil {
			op = "DELETE"
		}
		if MongoConnected && MongoDB != nil {
			if _, err := appendSyncChangeMongo(ctx, op, syncKind, p.Key, p.After); err != nil {
				return err
			}
		} else {
			memMu.Lock()
			appendSyncChangeMemoryLocked(op, syncKind, p.Key, p.After)
			memMu.Unlock()
		}
	}
	if MongoConnected && MongoDB != nil {
		_, err := MongoDB.Collection(colCurationPending).DeleteOne(ctx, bson.M{"_id": p.ID})
		return err
	}
	return nil
}

// ExportAdminDataset constructs an allowlist DTO. No user-submitted free text,
// reporter/submitter IDs, owner/contact data, session fields or tokens are exported.
func ExportAdminDataset(ctx context.Context, dataset string) ([]map[string]interface{}, error) {
	out := []map[string]interface{}{}
	switch dataset {
	case "verified_numbers":
		rows, e := ListAdminCuration(ctx, "verified")
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			v := map[string]interface{}{}
			for _, k := range []string{"phone_e164", "kind", "display_label", "source_url", "verified_at", "expires_at", "status"} {
				v[k] = r[k]
			}
			out = append(out, v)
		}
		return out, nil
	case "phone_tags", "moderation_cases":
	default:
		return nil, errors.New("unsupported export dataset")
	}
	if MongoConnected && MongoDB != nil {
		col := ColPhoneTags
		if dataset == "moderation_cases" {
			col = ColModerationCases
		}
		cur, e := MongoDB.Collection(col).Find(ctx, bson.M{}, options.Find().SetLimit(10000))
		if e != nil {
			return nil, e
		}
		defer cur.Close(ctx)
		if dataset == "phone_tags" {
			var rows []model.PhoneTag
			if e = cur.All(ctx, &rows); e != nil {
				return nil, e
			}
			for _, v := range rows {
				out = append(out, safePhoneExport(v))
			}
		} else {
			var rows []model.ModerationCase
			if e = cur.All(ctx, &rows); e != nil {
				return nil, e
			}
			for _, v := range rows {
				out = append(out, safeCaseExport(v))
			}
		}
		return out, nil
	}
	if IsProdEnv() {
		return nil, errors.New("MongoDB required")
	}
	memMu.RLock()
	defer memMu.RUnlock()
	if dataset == "phone_tags" {
		for _, v := range memPhoneTags {
			out = append(out, safePhoneExport(v))
		}
	} else {
		for _, v := range memModerationCases {
			out = append(out, safeCaseExport(v))
		}
	}
	return out, nil
}
func safePhoneExport(v model.PhoneTag) map[string]interface{} {
	return map[string]interface{}{"phone_e164": v.PhoneE164, "reputation_level": v.ReputationLevel, "risk_score": v.RiskScore, "primary_category": v.PrimaryCategory, "total_reports": v.TotalReports, "unique_reporters": v.UniqueReporters, "updated_at": v.UpdatedAt}
}
func safeCaseExport(v model.ModerationCase) map[string]interface{} {
	return map[string]interface{}{"dispute_id": v.DisputeID, "type": v.Type, "phone_e164": v.PhoneE164, "status": v.Status, "version": v.Version, "submitted_at": v.SubmittedAt, "updated_at": v.UpdatedAt}
}

type adminImportPending struct {
	ID     primitive.ObjectID `bson:"_id"`
	Actor  string             `bson:"actor"`
	Reason string             `bson:"reason"`
	Kind   string             `bson:"kind"`
	Rows   []dataset.Record   `bson:"rows"`
}

const colAdminImportPending = "admin_import_pending"

// ImportAdminDataset keeps a durable batch receipt until imports and audit have
// completed. The existing parser/importer validates and deduplicates all rows.
func ImportAdminDataset(ctx context.Context, rows []dataset.Record, kind, actor, reason string, dry bool) (ImportResult, error) {
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" {
		return ImportResult{}, errors.New("actor and reason required")
	}
	validation, err := ImportDataset(ctx, rows, kind, true)
	if err != nil || dry {
		return validation, err
	}
	p := adminImportPending{ID: primitive.NewObjectID(), Actor: actor, Reason: reason, Kind: kind, Rows: rows}
	if MongoConnected && MongoDB != nil {
		if _, err = MongoDB.Collection(colAdminImportPending).InsertOne(ctx, p); err != nil {
			return validation, err
		}
	}
	return finishAdminImport(ctx, p)
}
func finishAdminImport(ctx context.Context, p adminImportPending) (ImportResult, error) {
	var result ImportResult
	var err error
	if p.Kind == "verified" {
		result.Validated = len(p.Rows)
		for _, r := range p.Rows {
			v := model.VerifiedNumber{PhoneE164: r.PhoneE164, Kind: r.Kind, DisplayLabel: r.DisplayLabel, SourceURL: r.SourceURL, VerifiedBy: p.Actor, VerifiedAt: r.VerifiedAt, ExpiresAt: r.ExpiresAt, Status: "ACTIVE"}
			changed, e := UpsertVerifiedNumber(ctx, v)
			if e != nil {
				return result, e
			}
			if changed {
				result.Written++
			} else {
				result.Skipped++
			}
		}
	} else {
		result, err = ImportDataset(ctx, p.Rows, p.Kind, false)
		if err != nil {
			return result, err
		}
	}
	summary := map[string]interface{}{"batch_id": p.ID.Hex(), "kind": p.Kind, "validated": len(p.Rows)}
	if err = RecordAdminAudit(ctx, p.Actor, "DATASET_IMPORT", p.Kind+":"+p.ID.Hex(), p.Reason, nil, summary); err != nil {
		return result, err
	}
	if MongoConnected && MongoDB != nil {
		_, err = MongoDB.Collection(colAdminImportPending).DeleteOne(ctx, bson.M{"_id": p.ID})
	}
	return result, err
}
func RecoverAdminImports(ctx context.Context) error {
	if !MongoConnected || MongoDB == nil {
		return nil
	}
	cur, err := MongoDB.Collection(colAdminImportPending).Find(ctx, bson.M{})
	if err != nil {
		return err
	}
	defer cur.Close(ctx)
	var rows []adminImportPending
	if err = cur.All(ctx, &rows); err != nil {
		return err
	}
	for _, p := range rows {
		if _, err = finishAdminImport(ctx, p); err != nil {
			return err
		}
	}
	return nil
}
