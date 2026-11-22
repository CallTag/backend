package config

import (
	"context"
	"errors"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrCaseConflict = errors.New("case version conflict")
var ErrInvalidDecision = errors.New("invalid moderation decision")
var moderationMu sync.Mutex

func sortAudit(out []model.AuditLog) {
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
}
func ResetModerationMemoryLocked() { memAdminAudit = map[primitive.ObjectID]model.AuditLog{} }
func EnsureModerationIndexes(ctx context.Context) error {
	if !MongoConnected || MongoDB == nil {
		return nil
	}
	_, err := MongoDB.Collection(ColModerationCases).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}, {Key: "type", Value: 1}, {Key: "submitted_at", Value: 1}}})
	return err
}
func adminCase(ctx context.Context, id string) (*model.ModerationCase, error) {
	if MongoConnected && MongoDB != nil {
		var c model.ModerationCase
		f := bson.M{"dispute_id": id}
		if oid, e := primitive.ObjectIDFromHex(id); e == nil {
			f = bson.M{"$or": bson.A{f, bson.M{"_id": oid}}}
		}
		err := MongoDB.Collection(ColModerationCases).FindOne(ctx, f).Decode(&c)
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = ErrNotFound
		}
		return &c, err
	}
	memMu.RLock()
	defer memMu.RUnlock()
	for _, c := range memModerationCases {
		if c.DisputeID == id || c.ID.Hex() == id {
			return &c, nil
		}
	}
	return nil, ErrNotFound
}
func GetAdminCase(ctx context.Context, id string) (model.AdminCaseDetail, error) {
	c, err := adminCase(ctx, id)
	if err != nil {
		return model.AdminCaseDetail{}, err
	}
	p, reps, err := GetAdminPhone(ctx, c.PhoneE164)
	return model.AdminCaseDetail{Case: *c, Phone: p, Reports: reps}, err
}
func openCase(s string) bool {
	return s == "OPEN" || s == "UNDER_REVIEW" || s == "NEEDS_INFO" || s == "PROCESSING"
}
func ListAdminCases(ctx context.Context, status, kind, query string, limit int) (model.AdminCaseQueue, error) {
	out := model.AdminCaseQueue{Items: []model.ModerationCase{}}
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	all := []model.ModerationCase{}
	now := time.Now().UTC()
	if MongoConnected && MongoDB != nil {
		cur, err := MongoDB.Collection(ColModerationCases).Find(ctx, bson.M{})
		if err != nil {
			return out, err
		}
		if err = cur.All(ctx, &all); err != nil {
			return out, err
		}
		n, err := MongoDB.Collection(ColSpamReports).CountDocuments(ctx, bson.M{"created_at": bson.M{"$gte": now.Add(-24 * time.Hour)}})
		if err != nil {
			return out, err
		}
		out.Summary.Reports24h = int(n)
	} else {
		memMu.RLock()
		for _, c := range memModerationCases {
			all = append(all, c)
		}
		for _, reps := range memSpamReports {
			for _, r := range reps {
				if !r.CreatedAt.Before(now.Add(-24 * time.Hour)) {
					out.Summary.Reports24h++
				}
			}
		}
		memMu.RUnlock()
	}
	sort.Slice(all, func(i, j int) bool { return all[i].SubmittedAt.Before(all[j].SubmittedAt) })
	query = strings.ToLower(strings.TrimSpace(query))
	normalized := normalizeAdminPhoneSearch(query)
	for _, c := range all {
		if openCase(c.Status) {
			out.Summary.QueueCount++
			if c.Type == "DISPUTE" && now.Sub(c.SubmittedAt) > 7*24*time.Hour {
				out.Summary.OverdueDisputes++
			}
			if c.Type == "REPORT_REVIEW" && now.Sub(c.SubmittedAt) > 3*24*time.Hour {
				out.Summary.OverdueReports++
			}
		}
		matchesStatus := status == "" || c.Status == status || (status == "ACTIVE" && openCase(c.Status))
		matchesQuery := strings.Contains(c.PhoneE164, query) || strings.Contains(strings.ToLower(c.DisputeID), query) ||
			(normalized != "" && strings.Contains(c.PhoneE164, normalized))
		if matchesStatus && matchesQuery && (kind == "" || c.Type == kind) && len(out.Items) < limit {
			out.Items = append(out.Items, c)
		}
	}
	return out, nil
}

// DecideAdminCase reserves a CAS and its recovery journal in the SAME document.
// Side effects are idempotent sets; a failed response can be retried with the
// original version and identical payload, including after process restart.
func DecideAdminCase(ctx context.Context, id string, version int, action, reason, actor string) (*model.ModerationCase, error) {
	moderationMu.Lock()
	defer moderationMu.Unlock()
	action = strings.ToUpper(strings.TrimSpace(action))
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 2000 || version < 1 {
		return nil, ErrInvalidDecision
	}
	c, err := adminCase(ctx, id)
	if err != nil {
		return nil, err
	}
	if d := c.Decision; d != nil && d.Version == version && d.Actor == actor && d.Action == action && d.Reason == reason {
		if !d.Done {
			if err = applyDecision(ctx, c); err != nil {
				return nil, err
			}
		}
		return c, nil
	}
	if c.Version != version || !openCase(c.Status) || c.Status == "PROCESSING" {
		return nil, ErrCaseConflict
	}
	if action != "ACCEPT" && action != "REJECT" && action != "NEEDS_INFO" && action != "CONFIRM_DANGER" {
		return nil, ErrInvalidDecision
	}
	if c.Type != "DISPUTE" && c.Type != "REPORT_REVIEW" {
		return nil, ErrInvalidDecision
	}
	p, reps, err := GetAdminPhone(ctx, c.PhoneE164)
	if err != nil {
		return nil, err
	}
	d := &model.AdminDecision{Actor: actor, Action: action, Reason: reason, Version: version, At: time.Now().UTC(), BeforeHash: auditHash(c)}
	dangerCase := c.SubmitterID == "system:scoring_gate"
	if action == "CONFIRM_DANGER" {
		if c.Type != "REPORT_REVIEW" || !dangerCase || p == nil {
			return nil, ErrInvalidDecision
		}
		fresh := helper.AggregatePhoneTag(p, reps, p.SyncVersion+1)
		if fresh.RiskScore < 70 || fresh.UniqueReporters < 5 || fresh.IsWhitelistedVerified {
			return nil, ErrInvalidDecision
		}
	}
	if dangerCase && action == "ACCEPT" {
		return nil, ErrInvalidDecision
	}
	for _, r := range reps {
		if c.Type == "DISPUTE" && action == "ACCEPT" && !r.CreatedAt.After(c.SubmittedAt) {
			d.ReportIDs = append(d.ReportIDs, r.ID)
		}
		if c.Type == "REPORT_REVIEW" && !dangerCase && (action == "ACCEPT" || action == "REJECT") && r.Status == "PENDING_MOD" {
			d.ReportIDs = append(d.ReportIDs, r.ID)
		}
	}
	if c.Type == "REPORT_REVIEW" && !dangerCase && (action == "ACCEPT" || action == "REJECT") && len(d.ReportIDs) == 0 {
		return nil, ErrInvalidDecision
	}
	if MongoConnected && MongoDB != nil {
		res, err := MongoDB.Collection(ColModerationCases).UpdateOne(ctx, bson.M{"_id": c.ID, "version": version, "status": c.Status}, bson.M{"$set": bson.M{"admin_decision": d, "status": "PROCESSING", "updated_at": d.At}, "$inc": bson.M{"version": 1}})
		if err != nil {
			return nil, err
		}
		if res.MatchedCount == 0 {
			return nil, ErrCaseConflict
		}
	}
	c.Decision = d
	c.Version++
	c.Status = "PROCESSING"
	c.UpdatedAt = d.At
	if !MongoConnected || MongoDB == nil {
		memMu.Lock()
		memModerationCases[c.DisputeID] = *c
		memMu.Unlock()
	}
	if err = applyDecision(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func decisionAudit(c *model.ModerationCase) model.AuditLog {
	d := c.Decision
	raw := auditHash([]interface{}{c.DisputeID, d.Version})
	oid, _ := primitive.ObjectIDFromHex(raw[:24])
	return model.AuditLog{ID: oid, Actor: d.Actor, Action: d.Action, Target: c.DisputeID, Reason: d.Reason, BeforeHash: d.BeforeHash, AfterHash: auditHash(map[string]interface{}{"case_id": c.DisputeID, "status": decisionStatus(d.Action), "version": c.Version, "report_ids": d.ReportIDs}), CreatedAt: d.At}
}
func decisionStatus(action string) string {
	switch action {
	case "ACCEPT", "CONFIRM_DANGER":
		return "ACCEPTED"
	case "REJECT":
		return "REJECTED"
	default:
		return "NEEDS_INFO"
	}
}
func applyDecision(ctx context.Context, c *model.ModerationCase) error {
	d := c.Decision
	// Append-only intent audit precedes writes to evidence/reputation.
	if err := recordAudit(ctx, decisionAudit(c)); err != nil {
		return err
	}
	status := "REJECTED"
	if c.Type == "REPORT_REVIEW" && d.Action == "ACCEPT" {
		status = "VALIDATED"
	}
	if MongoConnected && MongoDB != nil {
		if len(d.ReportIDs) > 0 {
			if _, err := MongoDB.Collection(ColSpamReports).UpdateMany(ctx, bson.M{"_id": bson.M{"$in": d.ReportIDs}, "phone_e164": c.PhoneE164}, bson.M{"$set": bson.M{"status": status}}); err != nil {
				return err
			}
		}
		if d.Action == "CONFIRM_DANGER" || (c.Type == "DISPUTE" && d.Action == "ACCEPT") {
			if _, err := MongoDB.Collection(ColPhoneTags).UpdateOne(ctx, bson.M{"phone_e164": c.PhoneE164}, bson.M{"$set": bson.M{"moderator_confirmed_danger": d.Action == "CONFIRM_DANGER"}}); err != nil {
				return err
			}
		}
		// Always re-read current reports: a restart never replays a stale score snapshot.
		if _, err := recomputePhoneTagMongo(ctx, MongoDB.Collection(ColSpamReports), MongoDB.Collection(ColPhoneTags), c.PhoneE164); err != nil {
			return err
		}
		// Acknowledgement follows durable audit + sync. Replayed UPSERT/DELETE events
		// are intentionally permitted: Android cursor processing is idempotent.
		_, err := MongoDB.Collection(ColModerationCases).UpdateOne(ctx, bson.M{"_id": c.ID, "version": c.Version}, bson.M{"$set": bson.M{"status": decisionStatus(d.Action), "resolution_notes": d.Reason, "admin_decision.done": true}})
		if err != nil {
			return err
		}
	} else {
		memMu.Lock()
		ids := map[primitive.ObjectID]bool{}
		for _, id := range d.ReportIDs {
			ids[id] = true
		}
		reps := memSpamReports[c.PhoneE164]
		for i := range reps {
			if ids[reps[i].ID] {
				reps[i].Status = status
			}
		}
		memSpamReports[c.PhoneE164] = reps
		p := memPhoneTags[c.PhoneE164]
		p.PhoneE164 = c.PhoneE164
		if d.Action == "CONFIRM_DANGER" || (c.Type == "DISPUTE" && d.Action == "ACCEPT") {
			p.ModeratorConfirmedDanger = d.Action == "CONFIRM_DANGER"
		}
		memSyncCounter++
		p = helper.AggregatePhoneTag(&p, reps, memSyncCounter)
		memPhoneTags[c.PhoneE164] = p
		op := "UPSERT"
		if p.TotalReports == 0 && !p.IsOfficialBlacklisted {
			op = "DELETE"
		}
		appendSyncChangeMemoryLocked(op, "PHONE_TAG", c.PhoneE164, buildPhoneTagSyncPayload(p))
		memMu.Unlock()
	}
	c.Status = decisionStatus(d.Action)
	c.ResolutionNotes = d.Reason
	d.Done = true
	if !MongoConnected || MongoDB == nil {
		memMu.Lock()
		memModerationCases[c.DisputeID] = *c
		memMu.Unlock()
	}
	return nil
}
func RecoverModerationDecisions(ctx context.Context) error {
	if !MongoConnected || MongoDB == nil {
		return nil
	}
	moderationMu.Lock()
	defer moderationMu.Unlock()
	cur, err := MongoDB.Collection(ColModerationCases).Find(ctx, bson.M{"admin_decision.done": false})
	if err != nil {
		return err
	}
	var cases []model.ModerationCase
	if err = cur.All(ctx, &cases); err != nil {
		return err
	}
	for i := range cases {
		if err = applyDecision(ctx, &cases[i]); err != nil {
			return err
		}
	}
	return nil
}

// GetAdminPhone reads the complete evidence set, including held reports and
// reporter identities needed for unique-reporter thresholds. Never use the
// public ten-report/redacted preview for moderation decisions.
func GetAdminPhone(ctx context.Context, phone string) (*model.PhoneTag, []model.SpamReport, error) {
	if MongoConnected && MongoDB != nil {
		var p model.PhoneTag
		err := MongoDB.Collection(ColPhoneTags).FindOne(ctx, bson.M{"phone_e164": phone}).Decode(&p)
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil, err
		}
		var ptr *model.PhoneTag
		if err == nil {
			ptr = &p
		}
		cur, err := MongoDB.Collection(ColSpamReports).Find(ctx, bson.M{"phone_e164": phone})
		if err != nil {
			return nil, nil, err
		}
		reports := []model.SpamReport{}
		err = cur.All(ctx, &reports)
		return ptr, reports, err
	}
	memMu.RLock()
	defer memMu.RUnlock()
	p, ok := memPhoneTags[phone]
	var ptr *model.PhoneTag
	if ok {
		ptr = &p
	}
	reps := append([]model.SpamReport{}, memSpamReports[phone]...)
	return ptr, reps, nil
}

func GetAdminPhoneHistory(ctx context.Context, phone string) ([]model.ModerationCase, []model.AuditLog, error) {
	cases := []model.ModerationCase{}
	audits := []model.AuditLog{}
	if MongoConnected && MongoDB != nil {
		cur, err := MongoDB.Collection(ColModerationCases).Find(ctx, bson.M{"phone_e164": phone})
		if err != nil {
			return nil, nil, err
		}
		if err = cur.All(ctx, &cases); err != nil {
			return nil, nil, err
		}
		targets := []string{helper.HashIdentity(phone)}
		for _, c := range cases {
			targets = append(targets, c.DisputeID)
		}
		cur, err = MongoDB.Collection(ColAuditLogs).Find(ctx, bson.M{"target": bson.M{"$in": targets}})
		if err != nil {
			return nil, nil, err
		}
		err = cur.All(ctx, &audits)
		sortAudit(audits)
		return cases, audits, err
	}
	memMu.RLock()
	defer memMu.RUnlock()
	targets := map[string]bool{helper.HashIdentity(phone): true}
	for _, c := range memModerationCases {
		if c.PhoneE164 == phone {
			cases = append(cases, c)
			targets[c.DisputeID] = true
		}
	}
	for _, a := range memAdminAudit {
		if targets[a.Target] {
			audits = append(audits, a)
		}
	}
	sortAudit(audits)
	return cases, audits, nil
}
