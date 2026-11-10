package seeds

import (
	"embed"
	"encoding/json"
	"time"

	"github.com/CallTag/backend/model"
)

//go:embed emergency_seed.json official_channels_seed.json scammers_seed.json
var seedFS embed.FS

// LoadEmergencySeeds memuat data awal Nomor Darurat Nasional & Lokal dari emergency_seed.json.
func LoadEmergencySeeds() ([]model.EmergencyDirectory, error) {
	raw, err := seedFS.ReadFile("emergency_seed.json")
	if err != nil {
		return nil, err
	}
	var items []model.EmergencyDirectory
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range items {
		if items[i].VerifiedAt.IsZero() {
			items[i].VerifiedAt = now
		}
	}
	return items, nil
}

// LoadOfficialChannelSeeds memuat data awal Pusat Panduan Lapor Resmi dari official_channels_seed.json.
func LoadOfficialChannelSeeds() ([]model.OfficialChannel, error) {
	raw, err := seedFS.ReadFile("official_channels_seed.json")
	if err != nil {
		return nil, err
	}
	var items []model.OfficialChannel
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range items {
		items[i].UpdatedAt = now
	}
	return items, nil
}

// LoadScammerSeeds memuat data awal contoh nomor scammer Indonesia dari scammers_seed.json.
func LoadScammerSeeds() ([]model.PhoneTag, error) {
	raw, err := seedFS.ReadFile("scammers_seed.json")
	if err != nil {
		return nil, err
	}
	var items []model.PhoneTag
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range items {
		if items[i].RiskScore == 0 && items[i].SpamScore > 0 {
			items[i].RiskScore = items[i].SpamScore
		} else if items[i].SpamScore == 0 && items[i].RiskScore > 0 {
			items[i].SpamScore = items[i].RiskScore
		}
		items[i].FirstReportedAt = now.Add(-14 * 24 * time.Hour)
		items[i].LastReportedAt = now
		items[i].UpdatedAt = now
	}
	return items, nil
}
