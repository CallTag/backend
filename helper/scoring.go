package helper

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/CallTag/backend/model"
)

var (
	reEmailPII        = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	reAccountPII      = regexp.MustCompile(`\b\d{8,15}\b`)
	reNIKPII          = regexp.MustCompile(`\b\d{16}\b`)
	rePhoneCommentPII = regexp.MustCompile(`(^|[^\w])(\+62|62|0)[\s\-]?8(?:\d[\s\-]?){6,11}\d\b`)
)

// MaskPhone menyamarkan nomor telepon E.164 untuk keperluan log server agar tidak membocorkan PII (T-106).
// Contoh: "+6281234567005" -> "+62812****7005".
func MaskPhone(phoneE164 string) string {
	p := strings.TrimSpace(phoneE164)
	if p == "" {
		return ""
	}
	if len(p) <= 6 {
		return "***"
	}
	if len(p) < 10 {
		return p[:3] + "****" + p[len(p)-2:]
	}
	return p[:6] + "****" + p[len(p)-4:]
}

// SanitizeCommentPII menyaring pola email, NIK 16 digit, dan nomor telepon di dalam komentar laporan publik (T-106).
func SanitizeCommentPII(comment string) string {
	cleaned := strings.TrimSpace(comment)
	if cleaned == "" {
		return ""
	}
	cleaned = reEmailPII.ReplaceAllString(cleaned, "[REDACTED_EMAIL]")
	cleaned = reNIKPII.ReplaceAllString(cleaned, "[REDACTED_NIK]")
	cleaned = rePhoneCommentPII.ReplaceAllString(cleaned, "${1}[REDACTED_PHONE]")
	cleaned = reAccountPII.ReplaceAllString(cleaned, "[REDACTED_NUMBER]")
	return cleaned
}

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
	model.CategoryInfoTag: {Code: model.CategoryInfoTag, Label: "Informasi Layanan / Peran", Weight: 0},
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

// NormalizePhoneID menormalisasi nomor Indonesia ke E.164 dan mempertahankan kode negara eksplisit.
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

	// Kode negara eksplisit harus dipertahankan sebelum normalisasi seluler 8xx.
	if strings.HasPrefix(trimmed, "+") {
		return "+" + d
	}

	// Nomor layanan nasional 150xxxx dipanggil tanpa awalan +62.
	if len(d) == 7 && strings.HasPrefix(d, "150") {
		return d
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

// DetectIndonesianCarrier memberi konteks prefix seluler, wilayah PSTN, atau negara internasional.
func DetectIndonesianCarrier(phoneE164 string) string {
	return detectCarrierContext(phoneE164)
}

// ReportMaxAgeDays adalah batas maksimal umur laporan aktif (12 bulan / 365 hari) sesuai T-206.
const ReportMaxAgeDays = 365.0

// MaskPhoneInPath menyamarkan nomor telepon pada path URL seperti /api/v1/phones/+6281234567005/reports (T-203).
func MaskPhoneInPath(path string) string {
	const prefix = "/api/v1/phones/"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	rest := strings.TrimPrefix(path, prefix)
	slashIdx := strings.Index(rest, "/")
	if slashIdx < 0 {
		return prefix + MaskPhone(rest)
	}
	rawPhone := rest[:slashIdx]
	suffix := rest[slashIdx:]
	return prefix + MaskPhone(rawPhone) + suffix
}

// ClampTrustWeight membatasi trust_weight pelapor pada rentang [0.25, 1.5] dengan default 1.0 bila <= 0 (T-202).
func ClampTrustWeight(trust float64) float64 {
	if trust <= 0 {
		return 1.0
	}
	if trust < 0.25 {
		return 0.25
	}
	if trust > 1.5 {
		return 1.5
	}
	return trust
}

// DetermineReputationLevelWithGate menetapkan status bahaya beserta status pending review moderator untuk DANGER_SCAM (OD-03, T-202).
func DetermineReputationLevelWithGate(score float64, uniqueReporters int, isOfficialBlacklisted, isWhitelisted, isModeratorConfirmed bool) (model.ReputationLevel, bool) {
	if isWhitelisted {
		return model.ReputationClean, false
	}
	if isOfficialBlacklisted {
		return model.ReputationDangerScam, false
	}
	if score >= 70.0 && uniqueReporters >= 5 {
		if isModeratorConfirmed {
			return model.ReputationDangerScam, false
		}
		return model.ReputationSpam, true
	}
	if score >= 40.0 && uniqueReporters >= 3 {
		return model.ReputationSpam, false
	}
	if score >= 15.0 && uniqueReporters >= 2 {
		return model.ReputationSuspicious, false
	}
	return model.ReputationClean, false
}

// DetermineReputationLevel menetapkan status bahaya sesuai ambang batas PRD yang telah dikalibrasi (tanpa bypass N >= 2).
func DetermineReputationLevel(score float64, uniqueReporters int, isOfficialBlacklisted bool, isWhitelisted bool) model.ReputationLevel {
	level, _ := DetermineReputationLevelWithGate(score, uniqueReporters, isOfficialBlacklisted, isWhitelisted, isOfficialBlacklisted)
	return level
}

// AggregatePhoneTag menghitung ulang reputasi nomor telepon memakai waktu UTC saat ini.
func AggregatePhoneTag(existing *model.PhoneTag, reports []model.SpamReport, nextSyncVersion int64) model.PhoneTag {
	return AggregatePhoneTagWithTime(existing, reports, nextSyncVersion, time.Now().UTC())
}

// AggregatePhoneTagWithTime menghitung ulang Risk Score, kategori dominan deterministik, daftar tag, gerbang OD-03,
// dan kedaluwarsa 12 bulan (365 hari) berdasarkan waktu referensi now (T-202, T-206).
func AggregatePhoneTagWithTime(existing *model.PhoneTag, reports []model.SpamReport, nextSyncVersion int64, now time.Time) model.PhoneTag {
	now = now.UTC()
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
	bestPerReporter := make(map[string]float64)
	var activeReportCount int
	var firstAt, lastAt time.Time

	for _, rep := range reports {
		if rep.Status == "REJECTED" || rep.Status == "PENDING_MOD" {
			continue
		}
		daysElapsed := now.Sub(rep.CreatedAt).Hours() / 24.0
		if daysElapsed > ReportMaxAgeDays {
			continue
		}
		if daysElapsed < 0 {
			daysElapsed = 0
		}

		activeReportCount++
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

		trust := ClampTrustWeight(rep.ReporterTrustSnapshot)
		decay := math.Pow(0.5, daysElapsed/HalfLifeDays)
		contrib := BaseReportPoint * meta.Weight * trust * decay
		if contrib > bestPerReporter[reporterKey] {
			bestPerReporter[reporterKey] = contrib
		}

		if firstAt.IsZero() || rep.CreatedAt.Before(firstAt) {
			firstAt = rep.CreatedAt
		}
		if lastAt.IsZero() || rep.CreatedAt.After(lastAt) {
			lastAt = rep.CreatedAt
		}
	}

	var rawScore float64
	for _, c := range bestPerReporter {
		rawScore += c
	}

	finalScore := math.Min(100.0, math.Round(rawScore*100)/100)
	uniqueReporters := len(bestPerReporter)

	// Urutkan tag berdasarkan jumlah vote terbanyak lalu abjad
	tags := make([]model.TagCount, 0, len(tagMap))
	for t, c := range tagMap {
		tags = append(tags, model.TagCount{Tag: t, Count: c})
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Count == tags[j].Count {
			return tags[i].Tag < tags[j].Tag
		}
		return tags[i].Count > tags[j].Count
	})

	// Cari kategori dominan secara deterministik (jumlah -> bobot -> kode abjad)
	primaryCat := model.CategoryTelemarketing
	primaryMeta, _ := GetCategoryInfo(primaryCat)
	if activeReportCount == 0 {
		primaryCat = ""
		primaryMeta = CategoryMeta{}
	} else {
		type catCandidate struct {
			code   model.CategoryCode
			count  int
			weight float64
		}
		var candidates []catCandidate
		for codeStr, cnt := range categoryBreakdown {
			if cnt <= 0 || (codeStr == string(model.CategoryInfoTag) && len(bestPerReporter) > 0) {
				continue
			}
			m, _ := GetCategoryInfo(model.CategoryCode(codeStr))
			candidates = append(candidates, catCandidate{
				code:   model.CategoryCode(codeStr),
				count:  cnt,
				weight: m.Weight,
			})
		}
		if len(candidates) > 0 {
			sort.Slice(candidates, func(i, j int) bool {
				if candidates[i].count != candidates[j].count {
					return candidates[i].count > candidates[j].count
				}
				if candidates[i].weight != candidates[j].weight {
					return candidates[i].weight > candidates[j].weight
				}
				return string(candidates[i].code) < string(candidates[j].code)
			})
			primaryCat = candidates[0].code
			primaryMeta, _ = GetCategoryInfo(primaryCat)
		}
	}

	topLabel := primaryMeta.Label
	if len(tags) > 0 {
		topLabel = tags[0].Tag
	}

	isOfficialBlacklisted := false
	isWhitelisted := false
	isModeratorConfirmed := false
	if existing != nil {
		isOfficialBlacklisted = existing.IsOfficialBlacklisted
		isWhitelisted = existing.IsWhitelistedVerified
		isModeratorConfirmed = existing.ModeratorConfirmedDanger
	}

	repLevel, pendingDanger := DetermineReputationLevelWithGate(finalScore, uniqueReporters, isOfficialBlacklisted, isWhitelisted, isModeratorConfirmed)

	var expiresAt time.Time
	if !lastAt.IsZero() {
		expiresAt = lastAt.Add(time.Duration(ReportMaxAgeDays) * 24 * time.Hour)
	}

	result := model.PhoneTag{
		PhoneE164:                phoneE164,
		CountryCode:              "ID",
		Carrier:                  DetectIndonesianCarrier(phoneE164),
		ReputationLevel:          repLevel,
		RiskScore:                finalScore,
		SpamScore:                finalScore,
		PrimaryCategory:          primaryCat,
		PrimaryCategoryLabel:     primaryMeta.Label,
		IsDanger:                 repLevel == model.ReputationDangerScam || repLevel == model.ReputationSpam,
		ModeratorConfirmedDanger: isModeratorConfirmed,
		PendingDangerReview:      pendingDanger,
		TopCommunityLabel:        topLabel,
		Tags:                     tags,
		CategoryBreakdown:        categoryBreakdown,
		UniqueReporters:          uniqueReporters,
		TotalReports:             activeReportCount,
		IsOfficialBlacklisted:    isOfficialBlacklisted,
		IsWhitelistedVerified:    isWhitelisted,
		SyncVersion:              nextSyncVersion,
		FirstReportedAt:          firstAt,
		LastReportedAt:           lastAt,
		LastActivityAt:           lastAt,
		ExpiresAt:                expiresAt,
		UpdatedAt:                now,
	}
	if existing != nil && !existing.ID.IsZero() {
		result.ID = existing.ID
	}
	return result
}
