package helper

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/CallTag/backend/model"
)

// BaseReportPoint adalah poin dasar per laporan valid sesuai rumus PRD (B = 8.0).
const BaseReportPoint = 8.0

// HalfLifeDays adalah waktu paruh peluruhan skor dalam hari (90 hari).
const HalfLifeDays = 90.0

// CategoryMeta menyimpan label tampilan bahasa Indonesia dan bobot keparahan (W_cat).
type CategoryMeta struct {
	Code   model.CategoryCode
	Label  string
	Weight float64
}

var indonesiaTaxonomy = map[model.CategoryCode]CategoryMeta{
	model.CategoryPinjolDC: {
		Code:   model.CategoryPinjolDC,
		Label:  "Pinjol Ilegal & Debt Collector Meresahkan",
		Weight: 1.5,
	},
	model.CategoryAPKSniffing: {
		Code:   model.CategoryAPKSniffing,
		Label:  "Penipuan Modus File APK (Kurir/Undangan/Tilang)",
		Weight: 2.0,
	},
	model.CategoryImpersonation: {
		Code:   model.CategoryImpersonation,
		Label:  "Impersonasi / CS Palsu (Bank/E-Wallet/Pajak/Bea Cukai)",
		Weight: 1.8,
	},
	model.CategoryJudol: {
		Code:   model.CategoryJudol,
		Label:  "Promosi Judi Online (Judol)",
		Weight: 1.3,
	},
	model.CategorySocialEng: {
		Code:   model.CategorySocialEng,
		Label:  "Penipuan Sosial / Hipnotis Telepon",
		Weight: 1.8,
	},
	model.CategoryTelemarketing: {
		Code:   model.CategoryTelemarketing,
		Label:  "Telemarketing / Spam Promosi Agresif",
		Weight: 1.0,
	},
}

// GetCategoryInfo mengembalikan metadata kategori dan status validitasnya.
func GetCategoryInfo(code model.CategoryCode) (CategoryMeta, bool) {
	normalized := model.CategoryCode(strings.ToUpper(strings.TrimSpace(string(code))))
	meta, ok := indonesiaTaxonomy[normalized]
	return meta, ok
}

// NormalizePhoneID menormalisasi nomor telepon Indonesia ke format E.164 (+62...).
// Nomor pendek darurat (seperti 112, 110, 113, 119, 14080) dipertahankan apa adanya.
func NormalizePhoneID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	var digits strings.Builder
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	if d == "" {
		return ""
	}

	// Nomor darurat pendek nasional (3-6 digit)
	if len(d) <= 6 {
		return d
	}

	if strings.HasPrefix(d, "0") {
		return "+62" + d[1:]
	}
	if strings.HasPrefix(d, "62") {
		return "+" + d
	}
	if strings.HasPrefix(d, "8") {
		return "+62" + d
	}
	return "+" + d
}

// DetectIndonesianCarrier mengenali operator seluler/telepon Indonesia berdasarkan prefix E.164.
func DetectIndonesianCarrier(phoneE164 string) string {
	prefixes := []struct {
		prefix  string
		carrier string
	}{
		{"+62811", "Telkomsel (Halo)"},
		{"+62812", "Telkomsel (Simpati/Halo)"},
		{"+62813", "Telkomsel (Simpati/Halo)"},
		{"+62821", "Telkomsel (Simpati)"},
		{"+62822", "Telkomsel (Loop/Simpati)"},
		{"+62823", "Telkomsel (AS)"},
		{"+62851", "Telkomsel (by.U / AS)"},
		{"+62852", "Telkomsel (AS)"},
		{"+62853", "Telkomsel (AS)"},
		{"+62814", "Indosat Ooredoo Hutchison"},
		{"+62815", "Indosat Ooredoo Hutchison (Mentari/Matrix)"},
		{"+62816", "Indosat Ooredoo Hutchison (IM3/Matrix)"},
		{"+62855", "Indosat Ooredoo Hutchison (Matrix)"},
		{"+62856", "Indosat Ooredoo Hutchison (IM3)"},
		{"+62857", "Indosat Ooredoo Hutchison (IM3)"},
		{"+62858", "Indosat Ooredoo Hutchison (Mentari)"},
		{"+62895", "Indosat Ooredoo Hutchison (Tri/3)"},
		{"+62896", "Indosat Ooredoo Hutchison (Tri/3)"},
		{"+62897", "Indosat Ooredoo Hutchison (Tri/3)"},
		{"+62898", "Indosat Ooredoo Hutchison (Tri/3)"},
		{"+62899", "Indosat Ooredoo Hutchison (Tri/3)"},
		{"+62817", "XL Axiata"},
		{"+62818", "XL Axiata"},
		{"+62819", "XL Axiata"},
		{"+62859", "XL Axiata"},
		{"+62877", "XL Axiata"},
		{"+62878", "XL Axiata"},
		{"+62831", "XL Axiata (AXIS)"},
		{"+62832", "XL Axiata (AXIS)"},
		{"+62833", "XL Axiata (AXIS)"},
		{"+62838", "XL Axiata (AXIS)"},
		{"+62881", "Smartfren"},
		{"+62882", "Smartfren"},
		{"+62883", "Smartfren"},
		{"+62884", "Smartfren"},
		{"+62885", "Smartfren"},
		{"+62886", "Smartfren"},
		{"+62887", "Smartfren"},
		{"+62888", "Smartfren"},
		{"+62889", "Smartfren"},
		{"+6221", "PSTN Telkom (Jabodetabek)"},
		{"+6222", "PSTN Telkom (Bandung)"},
		{"+6224", "PSTN Telkom (Semarang)"},
		{"+62274", "PSTN Telkom (Yogyakarta)"},
		{"+6231", "PSTN Telkom (Surabaya)"},
		{"+6261", "PSTN Telkom (Medan)"},
		{"+62411", "PSTN Telkom (Makassar)"},
		{"+62361", "PSTN Telkom (Denpasar)"},
	}

	for _, item := range prefixes {
		if strings.HasPrefix(phoneE164, item.prefix) {
			return item.carrier
		}
	}
	if len(phoneE164) <= 6 {
		return "Layanan Darurat Nasional Indonesia"
	}
	return "Operator Telekomunikasi Indonesia"
}

// DetermineReputationLevel menetapkan status bahaya sesuai ambang batas (Threshold) pada PRD Bagian 4.3.
func DetermineReputationLevel(score float64, uniqueReporters int, isOfficialBlacklisted bool, isWhitelisted bool) model.ReputationLevel {
	if isWhitelisted {
		return model.ReputationClean
	}
	if isOfficialBlacklisted || (score >= 70.0 && uniqueReporters >= 5) {
		return model.ReputationDangerScam
	}
	if score >= 40.0 && uniqueReporters >= 3 {
		return model.ReputationSpam
	}
	if score >= 15.0 && uniqueReporters >= 2 {
		return model.ReputationSuspicious
	}
	// Jika skor sangat tinggi dari 1-2 laporan awal (misal APK_SNIFFING), minimal beri peringatan SUSPICIOUS
	if score >= 15.0 && uniqueReporters >= 1 {
		return model.ReputationSuspicious
	}
	return model.ReputationClean
}

// AggregatePhoneTag menghitung ulang Risk Score, kategori dominan, daftar tag nama, dan level reputasi
// dari daftar laporan SpamReport untuk suatu nomor telepon.
func AggregatePhoneTag(existing *model.PhoneTag, reports []model.SpamReport, nextSyncVersion int64) model.PhoneTag {
	now := time.Now().UTC()
	phoneE164 := ""
	if existing != nil {
		phoneE164 = existing.PhoneE164
	} else if len(reports) > 0 {
		phoneE164 = reports[0].PhoneE164
	}

	categoryBreakdown := map[string]int{
		string(model.CategoryPinjolDC):      0,
		string(model.CategoryAPKSniffing):   0,
		string(model.CategoryImpersonation): 0,
		string(model.CategoryJudol):         0,
		string(model.CategorySocialEng):     0,
		string(model.CategoryTelemarketing): 0,
	}

	tagMap := make(map[string]int)
	uniqueReporterSet := make(map[string]bool)
	var rawScore float64
	var firstAt, lastAt time.Time

	for _, rep := range reports {
		if rep.Status == "REJECTED" {
			continue
		}
		meta, ok := GetCategoryInfo(rep.CategoryCode)
		if !ok {
			meta = indonesiaTaxonomy[model.CategoryTelemarketing]
		}

		categoryBreakdown[string(meta.Code)]++

		tagClean := strings.TrimSpace(rep.CustomTag)
		if tagClean == "" {
			tagClean = strings.TrimSpace(rep.TagName)
		}
		if tagClean != "" {
			tagMap[tagClean]++
		}

		reporterKey := rep.ReporterUserID
		if reporterKey == "" {
			reporterKey = rep.ID.Hex()
		}

		// Deduplikasi kontribusi skor per pelapor unik
		if !uniqueReporterSet[reporterKey] {
			uniqueReporterSet[reporterKey] = true

			trust := rep.ReporterTrustSnapshot
			if trust <= 0 {
				trust = 1.0
			}

			daysElapsed := now.Sub(rep.CreatedAt).Hours() / 24.0
			if daysElapsed < 0 {
				daysElapsed = 0
			}
			decay := math.Pow(0.5, daysElapsed/HalfLifeDays)

			rawScore += BaseReportPoint * meta.Weight * trust * decay
		}

		if firstAt.IsZero() || rep.CreatedAt.Before(firstAt) {
			firstAt = rep.CreatedAt
		}
		if lastAt.IsZero() || rep.CreatedAt.After(lastAt) {
			lastAt = rep.CreatedAt
		}
	}

	finalScore := math.Min(100.0, math.Round(rawScore*10)/10)
	uniqueReporters := len(uniqueReporterSet)

	// Urutkan tag berdasarkan jumlah vote terbanyak
	var tags []model.TagCount
	for t, c := range tagMap {
		tags = append(tags, model.TagCount{Tag: t, Count: c})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count == tags[j].Count {
			return tags[i].Tag < tags[j].Tag
		}
		return tags[i].Count > tags[j].Count
	})

	// Cari kategori dominan
	primaryCat := model.CategoryTelemarketing
	maxCatCount := -1
	for codeStr, cnt := range categoryBreakdown {
		if cnt > maxCatCount {
			maxCatCount = cnt
			primaryCat = model.CategoryCode(codeStr)
		}
	}
	primaryMeta, _ := GetCategoryInfo(primaryCat)

	topLabel := primaryMeta.Label
	if len(tags) > 0 {
		topLabel = tags[0].Tag
	}

	isOfficialBlacklisted := false
	isWhitelisted := false
	if existing != nil {
		isOfficialBlacklisted = existing.IsOfficialBlacklisted
		isWhitelisted = existing.IsWhitelistedVerified
	}

	repLevel := DetermineReputationLevel(finalScore, uniqueReporters, isOfficialBlacklisted, isWhitelisted)

	result := model.PhoneTag{
		PhoneE164:             phoneE164,
		CountryCode:           "ID",
		Carrier:               DetectIndonesianCarrier(phoneE164),
		ReputationLevel:       repLevel,
		RiskScore:             finalScore,
		SpamScore:             finalScore,
		PrimaryCategory:       primaryCat,
		PrimaryCategoryLabel:  primaryMeta.Label,
		IsDanger:              repLevel == model.ReputationDangerScam || repLevel == model.ReputationSpam,
		TopCommunityLabel:     topLabel,
		Tags:                  tags,
		CategoryBreakdown:     categoryBreakdown,
		UniqueReporters:       uniqueReporters,
		TotalReports:          len(reports),
		IsOfficialBlacklisted: isOfficialBlacklisted,
		IsWhitelistedVerified: isWhitelisted,
		SyncVersion:           nextSyncVersion,
		FirstReportedAt:       firstAt,
		LastReportedAt:        lastAt,
		UpdatedAt:             now,
	}
	if existing != nil && !existing.ID.IsZero() {
		result.ID = existing.ID
	}
	return result
}
