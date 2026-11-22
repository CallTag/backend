package config

import (
	"context"
	"errors"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"testing"
	"time"
)

func TestModerationDisputeCorrectsAllReportsAndRetries(t *testing.T) {
	ResetMemoryForTest()
	ctx := context.Background()
	phone := "+6287771122334"
	now := time.Now().UTC()
	memMu.Lock()
	memAdminAudit = map[primitive.ObjectID]model.AuditLog{}
	reps := []model.SpamReport{}
	for i := 0; i < 16; i++ {
		reps = append(reps, model.SpamReport{ID: primitive.NewObjectID(), PhoneE164: phone, ReporterUserID: primitive.NewObjectID().Hex(), CategoryCode: model.CategoryAPKSniffing, Status: "VALIDATED", CreatedAt: now.Add(-time.Hour)})
	}
	p := model.PhoneTag{PhoneE164: phone, ModeratorConfirmedDanger: true}
	memSpamReports[phone] = reps
	memPhoneTags[phone] = helper.AggregatePhoneTag(&p, reps, 1)
	memMu.Unlock()
	c, _, err := CreateDisputeCase(ctx, model.DisputeRequest{Phone: phone, OwnerName: "Owner", Reason: "False reports"}, "test:owner")
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecideAdminCase(ctx, c.DisputeID, 1, "ACCEPT", "Evidence verified", "staff:1")
	if err != nil || got.Status != "ACCEPTED" || got.Version != 2 {
		t.Fatalf("decision %+v %v", got, err)
	}
	p2, reps, err := GetAdminPhone(ctx, phone)
	if err != nil || len(reps) != 16 || p2.TotalReports != 0 || p2.ModeratorConfirmedDanger {
		t.Fatalf("correction %+v reps=%d err=%v", p2, len(reps), err)
	}
	for _, r := range reps {
		if r.Status != "REJECTED" {
			t.Fatal("report remained active")
		}
	}
	audits, _ := ListAdminAudit(ctx, 100)
	if len(audits) != 1 || audits[0].BeforeHash == audits[0].AfterHash {
		t.Fatalf("audit %+v", audits)
	}
	if _, err = DecideAdminCase(ctx, c.DisputeID, 1, "ACCEPT", "Evidence verified", "staff:1"); err != nil {
		t.Fatal("identical retry", err)
	}
	if _, err = DecideAdminCase(ctx, c.DisputeID, 1, "REJECT", "Changed mind", "staff:1"); !errors.Is(err, ErrCaseConflict) {
		t.Fatalf("stale decision %v", err)
	}
	memMu.RLock()
	last := memSyncChanges[len(memSyncChanges)-1]
	memMu.RUnlock()
	if last.Op != "DELETE" || last.Key != phone {
		t.Fatalf("revocation %+v", last)
	}
}
func TestModerationDangerRequiresLiveUniqueReporters(t *testing.T) {
	ResetMemoryForTest()
	ctx := context.Background()
	phone := "+6287771122335"
	now := time.Now().UTC()
	c := model.ModerationCase{ID: primitive.NewObjectID(), DisputeID: "REV_test", PhoneE164: phone, Type: "REPORT_REVIEW", SubmitterID: "system:scoring_gate", Status: "OPEN", Version: 1, SubmittedAt: now}
	memMu.Lock()
	memModerationCases[c.DisputeID] = c
	memPhoneTags[phone] = model.PhoneTag{PhoneE164: phone, RiskScore: 100, UniqueReporters: 9, PendingDangerReview: true}
	memMu.Unlock()
	if _, err := DecideAdminCase(ctx, c.DisputeID, 1, "CONFIRM_DANGER", "Check live evidence", "staff"); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("stale cached candidate accepted: %v", err)
	}
	memMu.Lock()
	for i := 0; i < 5; i++ {
		memSpamReports[phone] = append(memSpamReports[phone], model.SpamReport{ID: primitive.NewObjectID(), ReporterUserID: primitive.NewObjectID().Hex(), PhoneE164: phone, CategoryCode: model.CategoryAPKSniffing, Status: "VALIDATED", CreatedAt: now})
	}
	memMu.Unlock()
	if _, err := DecideAdminCase(ctx, c.DisputeID, 1, "CONFIRM_DANGER", "Check live evidence", "staff"); err != nil {
		t.Fatal(err)
	}
	p, _, _ := GetAdminPhone(ctx, phone)
	if p.ReputationLevel != model.ReputationDangerScam {
		t.Fatalf("level %+v", p)
	}
}
func TestModerationSummarySLAAndRequiredReason(t *testing.T) {
	ResetMemoryForTest()
	ctx := context.Background()
	now := time.Now().UTC()
	memMu.Lock()
	memModerationCases = map[string]model.ModerationCase{"d": {DisputeID: "d", Type: "DISPUTE", Status: "UNDER_REVIEW", Version: 1, SubmittedAt: now.Add(-8 * 24 * time.Hour)}, "r": {DisputeID: "r", Type: "REPORT_REVIEW", Status: "OPEN", Version: 1, SubmittedAt: now.Add(-4 * 24 * time.Hour)}}
	memMu.Unlock()
	q, err := ListAdminCases(ctx, "OPEN", "REPORT_REVIEW", "", 1)
	if err != nil || len(q.Items) != 1 || q.Summary.QueueCount != 2 || q.Summary.OverdueDisputes != 1 || q.Summary.OverdueReports != 1 {
		t.Fatalf("queue %+v %v", q, err)
	}
	memMu.Lock()
	memModerationCases["closed"] = model.ModerationCase{DisputeID: "closed", Status: "ACCEPTED", SubmittedAt: now.Add(-10 * 24 * time.Hour)}
	memMu.Unlock()
	active, err := ListAdminCases(ctx, "ACTIVE", "", "", 1)
	if err != nil || len(active.Items) != 1 || active.Items[0].DisputeID != "d" || active.Summary.QueueCount != 2 {
		t.Fatalf("active queue %+v %v", active, err)
	}
	if _, err = DecideAdminCase(ctx, "d", 1, "ACCEPT", " ", "staff"); !errors.Is(err, ErrInvalidDecision) {
		t.Fatal(err)
	}
}
