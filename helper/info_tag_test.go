package helper

import (
	"github.com/CallTag/backend/model"
	"strings"
	"testing"
	"time"
)

func TestInfoTagNeverContributesRiskOrDangerQuorum(t *testing.T) {
	now := time.Now().UTC()
	var reports []model.SpamReport
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		reports = append(reports, model.SpamReport{PhoneE164: "+628111234567", ReporterUserID: id, CategoryCode: model.CategoryCode("INFO_TAG"), CustomTag: "Kurir Ekspedisi", CreatedAt: now})
	}
	got := AggregatePhoneTagWithTime(nil, reports, 1, now)
	if got.RiskScore != 0 || got.UniqueReporters != 0 || got.ReputationLevel != model.ReputationClean || got.PendingDangerReview {
		t.Fatalf("tag netral memengaruhi bahaya: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0].Count != 6 || got.TotalReports != 6 {
		t.Fatalf("tag konteks hilang: %+v", got)
	}
}

func TestInfoTagPIIFilterIncludesAccountNumbers(t *testing.T) {
	got := SanitizeCommentPII("Rekening 1234567890123, surel admin@example.com")
	if strings.Contains(got, "1234567890123") || strings.Contains(got, "admin@example.com") {
		t.Fatal("nomor rekening belum disaring")
	}
}

func TestInfoTagsDoNotReplaceSpamPrimaryCategory(t *testing.T) {
	now := time.Now().UTC()
	reports := []model.SpamReport{{PhoneE164: "+628111234567", ReporterUserID: "a", CategoryCode: model.CategoryAPKSniffing, CustomTag: "Modus APK", CreatedAt: now}}
	for _, id := range []string{"b", "c", "d", "e", "f"} {
		reports = append(reports, model.SpamReport{PhoneE164: "+628111234567", ReporterUserID: id, CategoryCode: model.CategoryInfoTag, CustomTag: "Kurir", CreatedAt: now})
	}
	got := AggregatePhoneTagWithTime(nil, reports, 1, now)
	if got.RiskScore != 16 || got.UniqueReporters != 1 || got.PrimaryCategory != model.CategoryAPKSniffing {
		t.Fatalf("konteks menimpa risiko: %+v", got)
	}
}

func TestPendingModerationReportsDoNotChangePublicReputation(t *testing.T) {
	now := time.Now().UTC()
	reports := []model.SpamReport{{PhoneE164: "+628111234567", ReporterUserID: "held", CategoryCode: model.CategoryAPKSniffing, CustomTag: "Belum ditinjau", Status: "PENDING_MOD", CreatedAt: now}}
	got := AggregatePhoneTagWithTime(nil, reports, 1, now)
	if got.RiskScore != 0 || got.TotalReports != 0 || len(got.Tags) != 0 {
		t.Fatalf("laporan tertahan memengaruhi reputasi: %+v", got)
	}
}
