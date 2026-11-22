package dataset

import (
	"os"
	"strings"
	"testing"
)

func TestParseCSVNormalizesDeduplicatesAndSanitizes(t *testing.T) {
	input := "phone_e164,category_code,custom_tag,comment,source\n081234567899,INFO_TAG,Kurir,email person@example.com,COMMUNITY\n+6281234567899,INFO_TAG,Kurir,email person@example.com,COMMUNITY\n"
	rows, err := Parse(strings.NewReader(input), ".csv", "tag_context", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].PhoneE164 != "+6281234567899" || strings.Contains(rows[0].Comment, "person@example.com") {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestParseRejectsInvalidRows(t *testing.T) {
	for _, input := range []string{
		`[{"phone_e164":"garbage","category_code":"INFO_TAG","custom_tag":"Kurir"}]`,
		`[{"phone_e164":"081234567899","category_code":"INVALID","custom_tag":"Kurir"}]`,
		`[{"phone_e164":"081234567899","category_code":"SOCIAL_ENG","custom_tag":"Kurir"}]`,
	} {
		if _, err := Parse(strings.NewReader(input), ".json", "tag_context", "TEAM"); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	if _, err := Parse(strings.NewReader(`[{"phone_e164":"1500888","kind":"BUSINESS","display_label":"Bank","source_url":"javascript:alert(1)"}]`), ".json", "verified", "TEAM"); err == nil {
		t.Fatal("accepted non-web provenance")
	}
}

func TestCallShieldFormat(t *testing.T) {
	rows, err := Parse(strings.NewReader(`{"version":1,"numbers":[{"number":"+12025550199","type":"scam","description":"Spam"}]}`), ".json", "scam", "EXTERNAL_OPEN_LIST")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Source != "EXTERNAL_OPEN_LIST" || rows[0].CategoryCode != "SOCIAL_ENG" || rows[0].CustomTag != "External open list: scam" || rows[0].Comment != "Spam" {
		t.Fatalf("unexpected: %+v", rows)
	}
}

func TestPinnedMITCallShieldFixture(t *testing.T) {
	f, err := os.Open("testdata/callshield_sample.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := Parse(f, ".json", "scam", "EXTERNAL_OPEN_LIST")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].PhoneE164 != "+13152328257" {
		t.Fatalf("unexpected upstream fixture: %+v", rows)
	}
	for _, row := range rows {
		if row.Source != "EXTERNAL_OPEN_LIST" {
			t.Fatal("lost provenance")
		}
	}
}

func TestVerifiedDatesAndHTTPSRequired(t *testing.T) {
	good := `[{"phone_e164":"1500888","kind":"BUSINESS","display_label":"Bank","source_url":"https://bank.example/contact","verified_at":"2026-11-16T00:00:00Z","expires_at":"2027-11-16T00:00:00Z"}]`
	rows, err := Parse(strings.NewReader(good), ".json", "verified", "TEAM")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].PhoneE164 != "1500888" {
		t.Fatalf("service number changed: %s", rows[0].PhoneE164)
	}
	for _, bad := range []string{strings.Replace(good, "https://", "http://", 1), strings.Replace(good, "2027-11-16", "2025-11-16", 1), strings.Replace(good, "1500888", "+0123456789", 1), strings.Replace(good, "1500888", "+123", 1)} {
		if _, err := Parse(strings.NewReader(bad), ".json", "verified", "TEAM"); err == nil {
			t.Fatal("accepted invalid provenance/phone")
		}
	}
}

func TestExternalListCannotGrantVerifiedStatus(t *testing.T) {
	input := `[{"phone_e164":"1500888","kind":"BUSINESS","display_label":"Bank","source_url":"https://bank.example/contact","verified_at":"2026-11-16T00:00:00Z","expires_at":"2027-11-16T00:00:00Z"}]`
	if _, err := Parse(strings.NewReader(input), ".json", "verified", "EXTERNAL_OPEN_LIST"); err == nil {
		t.Fatal("external list can grant verified shield")
	}
}
