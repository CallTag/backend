package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"strings"
	"time"
)

const ColAuditLogs = "audit_logs"

var memAdminAudit = map[primitive.ObjectID]model.AuditLog{}

func auditHash(v interface{}) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func RecordAdminAudit(ctx context.Context, actor, action, target, reason string, before, after interface{}) error {
	a := model.AuditLog{ID: primitive.NewObjectID(), Actor: actor, Action: action, Target: target, Reason: helper.SanitizeCommentPII(reason), BeforeHash: auditHash(before), AfterHash: auditHash(after), CreatedAt: time.Now().UTC()}
	return recordAudit(ctx, a)
}
func recordAudit(ctx context.Context, a model.AuditLog) error {
	a.Reason = helper.SanitizeCommentPII(a.Reason)
	if strings.HasPrefix(a.Target, "+") {
		a.Target = helper.HashIdentity(a.Target)
	}
	if MongoConnected && MongoDB != nil {
		_, err := MongoDB.Collection(ColAuditLogs).InsertOne(ctx, a)
		if mongo.IsDuplicateKeyError(err) {
			return nil
		}
		return err
	}
	memMu.Lock()
	defer memMu.Unlock()
	memAdminAudit[a.ID] = a
	return nil
}
func ListAdminAudit(ctx context.Context, limit int) ([]model.AuditLog, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	out := []model.AuditLog{}
	if MongoConnected && MongoDB != nil {
		cur, err := MongoDB.Collection(ColAuditLogs).Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
		if err != nil {
			return nil, err
		}
		err = cur.All(ctx, &out)
		return out, err
	}
	memMu.RLock()
	for _, a := range memAdminAudit {
		out = append(out, a)
	}
	memMu.RUnlock()
	sortAudit(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func EnsureAuditIndexes(ctx context.Context) error {
	if !MongoConnected || MongoDB == nil {
		return nil
	}
	_, err := MongoDB.Collection(ColAuditLogs).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "created_at", Value: -1}}})
	return err
}
