package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CategoryCode merepresentasikan 6 taksonomi deteksi spam & scam khas Indonesia sesuai PRD 4.1.
type CategoryCode string

const (
	CategoryPinjolDC      CategoryCode = "PINJOL_DC"
	CategoryAPKSniffing   CategoryCode = "APK_SNIFFING"
	CategoryImpersonation CategoryCode = "IMPERSONATION"
	CategoryJudol         CategoryCode = "JUDOL"
	CategorySocialEng     CategoryCode = "SOCIAL_ENG"
	CategoryTelemarketing CategoryCode = "TELEMARKETING"
)

// ReputationLevel merepresentasikan 4 tingkat bahaya nomor telepon sesuai PRD 4.2 & 4.3.
type ReputationLevel string

const (
	ReputationClean      ReputationLevel = "CLEAN"
	ReputationSuspicious ReputationLevel = "SUSPICIOUS"
	ReputationSpam       ReputationLevel = "SPAM"
	ReputationDangerScam ReputationLevel = "DANGER_SCAM"
)

// APIResponse adalah pembungkus standar respons JSON ala gocroot.
type APIResponse struct {
	Success   bool        `json:"success" bson:"success"`
	Code      int         `json:"code" bson:"code"`
	Message   string      `json:"message" bson:"message"`
	Data      interface{} `json:"data,omitempty" bson:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp" bson:"timestamp"`
}

// User merepresentasikan dokumen pengguna pada koleksi MongoDB "users".
type User struct {
	ID               primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	PhoneE164        string             `json:"phone_e164,omitempty" bson:"-"`
	PhoneHash        string             `json:"phone_hash" bson:"phone_hash"`
	DeviceIDHash     string             `json:"device_id_hash" bson:"device_id_hash"`
	DisplayName      string             `json:"display_name" bson:"display_name"`
	Role             string             `json:"role" bson:"role"`
	TrustWeight      float64            `json:"trust_weight" bson:"trust_weight"`
	AccuracyStats    AccuracyStats      `json:"accuracy_stats" bson:"accuracy_stats"`
	DailyReportCount int                `json:"daily_report_count" bson:"daily_report_count"`
	LastReportAt     time.Time          `json:"last_report_at,omitempty" bson:"last_report_at,omitempty"`
	Status           string             `json:"status" bson:"status"`
	CreatedAt        time.Time          `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" bson:"updated_at"`
}

// AccuracyStats menyimpan rekam jejak akurasi laporan pengguna.
type AccuracyStats struct {
	TotalReports     int `json:"total_reports" bson:"total_reports"`
	ConfirmedReports int `json:"confirmed_reports" bson:"confirmed_reports"`
	DisputedReports  int `json:"disputed_reports" bson:"disputed_reports"`
}

// TagCount menyimpan daftar tag nama komunitas beserta jumlah vote-nya.
type TagCount struct {
	Tag   string `json:"tag" bson:"tag"`
	Count int    `json:"count" bson:"count"`
}

// PhoneTag merepresentasikan dokumen agregasi reputasi nomor telepon pada koleksi "phone_tags".
type PhoneTag struct {
	ID                    primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	PhoneE164             string             `json:"phone_e164" bson:"phone_e164"`
	CountryCode           string             `json:"country_code" bson:"country_code"`
	Carrier               string             `json:"carrier" bson:"carrier"`
	ReputationLevel       ReputationLevel    `json:"reputation_level" bson:"reputation_level"`
	RiskScore             float64            `json:"risk_score" bson:"risk_score"`
	SpamScore             float64            `json:"spam_score" bson:"-"`
	PrimaryCategory       CategoryCode       `json:"primary_category" bson:"primary_category"`
	PrimaryCategoryLabel  string             `json:"primary_category_label" bson:"primary_category_label"`
	IsDanger              bool               `json:"is_danger" bson:"is_danger"`
	TopCommunityLabel     string             `json:"top_community_label" bson:"top_community_label"`
	Tags                  []TagCount         `json:"tags" bson:"tags"`
	CategoryBreakdown     map[string]int     `json:"category_breakdown" bson:"category_breakdown"`
	UniqueReporters       int                `json:"unique_reporters" bson:"unique_reporters"`
	TotalReports          int                `json:"total_reports" bson:"total_reports"`
	IsOfficialBlacklisted bool               `json:"is_official_blacklisted" bson:"is_official_blacklisted"`
	IsWhitelistedVerified bool               `json:"is_whitelisted_verified" bson:"is_whitelisted_verified"`
	SyncVersion           int64              `json:"sync_version" bson:"sync_version"`
	FirstReportedAt       time.Time          `json:"first_reported_at,omitempty" bson:"first_reported_at,omitempty"`
	LastReportedAt        time.Time          `json:"last_reported_at,omitempty" bson:"last_reported_at,omitempty"`
	UpdatedAt             time.Time          `json:"updated_at" bson:"updated_at"`
}

// SpamReport merepresentasikan dokumen laporan individual pada koleksi "spam_reports".
type SpamReport struct {
	ID                    primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	PhoneE164             string             `json:"phone_e164" bson:"phone_e164"`
	ReporterUserID        string             `json:"reporter_user_id" bson:"reporter_user_id"`
	CategoryCode          CategoryCode       `json:"category_code" bson:"category_code"`
	CustomTag             string             `json:"custom_tag" bson:"custom_tag"`
	TagName               string             `json:"tag_name" bson:"-"`
	Comment               string             `json:"comment" bson:"comment"`
	CallType              string             `json:"call_type" bson:"call_type"`
	ReporterTrustSnapshot float64            `json:"reporter_trust_snapshot" bson:"reporter_trust_snapshot"`
	AppliedWeight         float64            `json:"applied_weight" bson:"applied_weight"`
	Status                string             `json:"status" bson:"status"`
	CreatedAt             time.Time          `json:"created_at" bson:"created_at"`
}

// EmergencyContactDetails menyimpan kontak telepon PSTN/seluler dan WhatsApp instansi darurat.
type EmergencyContactDetails struct {
	ShortCode        string   `json:"short_code,omitempty" bson:"short_code,omitempty"`
	PhoneNumbers     []string `json:"phone_numbers" bson:"phone_numbers"`
	WhatsAppNumber   string   `json:"whatsapp_number,omitempty" bson:"whatsapp_number,omitempty"`
	WhatsAppTemplate string   `json:"whatsapp_template,omitempty" bson:"whatsapp_template,omitempty"`
}

// GeoPoint merepresentasikan koordinat GeoJSON untuk pencarian lokasi terdekat.
type GeoPoint struct {
	Type        string    `json:"type" bson:"type"`
	Coordinates []float64 `json:"coordinates" bson:"coordinates"` // [longitude, latitude]
}

// EmergencyDirectory merepresentasikan dokumen nomor darurat Nasional & Lokal pada koleksi "emergency_directory".
type EmergencyDirectory struct {
	ID           primitive.ObjectID      `json:"id" bson:"_id,omitempty"`
	Code         string                  `json:"code" bson:"code"`
	Scope        string                  `json:"scope" bson:"scope"`             // NATIONAL atau LOCAL
	AgencyType   string                  `json:"agency_type" bson:"agency_type"` // TERPADU, POLISI, DAMKAR, AMBULANS_RS, SAR, PLN, JASA_MARGA
	Name         string                  `json:"name" bson:"name"`
	Province     string                  `json:"province,omitempty" bson:"province,omitempty"`
	CityRegency  string                  `json:"city_regency,omitempty" bson:"city_regency,omitempty"`
	Contacts     EmergencyContactDetails `json:"contacts" bson:"contacts"`
	Capabilities []string                `json:"capabilities" bson:"capabilities"`
	Address      string                  `json:"address,omitempty" bson:"address,omitempty"`
	Location     *GeoPoint               `json:"location,omitempty" bson:"location,omitempty"`
	Is24Hours    bool                    `json:"is_24_hours" bson:"is_24_hours"`
	VerifiedAt   time.Time               `json:"verified_at" bson:"verified_at"`
	SyncVersion  int64                   `json:"sync_version" bson:"sync_version"`
}

// OfficialChannelLinks menyimpan tautan kanal pelaporan resmi pemerintah.
type OfficialChannelLinks struct {
	WebsiteURL  string `json:"website_url" bson:"website_url"`
	PhoneDial   string `json:"phone_dial,omitempty" bson:"phone_dial,omitempty"`
	WhatsAppURL string `json:"whatsapp_url,omitempty" bson:"whatsapp_url,omitempty"`
	Email       string `json:"email,omitempty" bson:"email,omitempty"`
}

// OfficialChannel merepresentasikan dokumen Pusat Panduan Lapor Resmi pada koleksi "official_channels".
type OfficialChannel struct {
	ID                        primitive.ObjectID   `json:"id" bson:"_id,omitempty"`
	ChannelSlug               string               `json:"channel_slug" bson:"channel_slug"`
	Title                     string               `json:"title" bson:"title"`
	AuthorityName             string               `json:"authority_name" bson:"authority_name"`
	RelatedCategories         []CategoryCode       `json:"related_categories" bson:"related_categories"`
	Description               string               `json:"description" bson:"description"`
	ActionLinks               OfficialChannelLinks `json:"action_links" bson:"action_links"`
	RequiredEvidenceChecklist []string             `json:"required_evidence_checklist" bson:"required_evidence_checklist"`
	StepByStepGuide           []string             `json:"step_by_step_guide" bson:"step_by_step_guide"`
	DisplayOrder              int                  `json:"display_order" bson:"display_order"`
	IsActive                  bool                 `json:"is_active" bson:"is_active"`
	UpdatedAt                 time.Time            `json:"updated_at" bson:"updated_at"`
}

// AuthLoginRequest adalah payload request untuk POST /api/v1/auth/device.
type AuthLoginRequest struct {
	Phone       string `json:"phone"`
	DisplayName string `json:"display_name"`
	DeviceID    string `json:"device_id"`
}

// AuthLoginResponse adalah payload response untuk POST /api/v1/auth/device.
type AuthLoginResponse struct {
	Token     string    `json:"token"`
	TokenType string    `json:"token_type"`
	ExpiresAt time.Time `json:"expires_at"`
	User      User      `json:"user"`
}

// ReportRequest adalah payload request untuk POST /api/v1/reports.
type ReportRequest struct {
	Phone        string       `json:"phone"`
	CategoryCode CategoryCode `json:"category_code"`
	CustomTag    string       `json:"custom_tag"`
	TagName      string       `json:"tag_name"`
	Comment      string       `json:"comment"`
	CallType     string       `json:"call_type"`
}

// ReportResponse adalah payload response setelah berhasil mengirim laporan ke POST /api/v1/reports.
type ReportResponse struct {
	ReportID               string            `json:"report_id"`
	PhoneTag               PhoneTag          `json:"phone_tag"`
	RecommendedOfficialHub []OfficialChannel `json:"recommended_official_hub"`
}

// LookupResponse adalah payload response untuk GET /api/v1/phones/{e164}.
type LookupResponse struct {
	PhoneE164             string          `json:"phone_e164"`
	Carrier               string          `json:"carrier"`
	Found                 bool            `json:"found"`
	ReputationLevel       ReputationLevel `json:"reputation_level"`
	RiskScore             float64         `json:"risk_score"`
	SpamScore             float64         `json:"spam_score"`
	IsDanger              bool            `json:"is_danger"`
	IsOfficialBlacklisted bool            `json:"is_official_blacklisted"`
	IsWhitelistedVerified bool            `json:"is_whitelisted_verified"`
	PrimaryCategory       CategoryCode    `json:"primary_category,omitempty"`
	PrimaryCategoryLabel  string          `json:"primary_category_label,omitempty"`
	TopCommunityLabel     string          `json:"top_community_label,omitempty"`
	Tags                  []TagCount      `json:"tags"`
	CategoryBreakdown     map[string]int  `json:"category_breakdown"`
	UniqueReporters       int             `json:"unique_reporters"`
	TotalReports          int             `json:"total_reports"`
	RecentReports         []SpamReport    `json:"recent_reports,omitempty"`
	LastReportedAt        *time.Time      `json:"last_reported_at,omitempty"`
}

// PhoneReportsResponse adalah payload response untuk GET /api/v1/phones/{e164}/reports.
type PhoneReportsResponse struct {
	PhoneE164         string          `json:"phone_e164"`
	ReputationLevel   ReputationLevel `json:"reputation_level"`
	RiskScore         float64         `json:"risk_score"`
	SpamScore         float64         `json:"spam_score"`
	TopCommunityLabel string          `json:"top_community_label,omitempty"`
	UniqueReporters   int             `json:"unique_reporters"`
	TotalReports      int             `json:"total_reports"`
	CategoryBreakdown map[string]int  `json:"category_breakdown"`
	Reports           []SpamReport    `json:"reports"`
	RecentReports     []SpamReport    `json:"recent_reports"`
}

// EmergencyResponse adalah payload response untuk GET /api/v1/emergency/national & /api/v1/emergency/local.
type EmergencyResponse struct {
	FilterProvince   string               `json:"filter_province,omitempty"`
	FilterCity       string               `json:"filter_city,omitempty"`
	FilterAgencyType string               `json:"filter_agency_type,omitempty"`
	NationalNumbers  []EmergencyDirectory `json:"national_numbers"`
	LocalDirectory   []EmergencyDirectory `json:"local_directory"`
	OfficialChannels []OfficialChannel    `json:"official_channels,omitempty"`
}

// SpamSyncItem adalah item ringkas untuk sinkronisasi Room DB / SQLite lokal di Android (/api/v1/sync/offline-bundle).
type SpamSyncItem struct {
	PhoneE164            string          `json:"phone_e164"`
	ReputationLevel      ReputationLevel `json:"reputation_level"`
	RiskScore            float64         `json:"risk_score"`
	SpamScore            float64         `json:"spam_score"`
	IsDanger             bool            `json:"is_danger"`
	PrimaryCategory      CategoryCode    `json:"primary_category"`
	PrimaryCategoryLabel string          `json:"primary_category_label"`
	TopCommunityLabel    string          `json:"top_community_label"`
	SyncVersion          int64           `json:"sync_version"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

// SpamSyncResponse adalah payload response untuk GET /api/v1/sync/offline-bundle.
type SpamSyncResponse struct {
	LatestSyncVersion int64                `json:"latest_sync_version"`
	Count             int                  `json:"count"`
	Items             []SpamSyncItem       `json:"items"`
	NationalEmergency []EmergencyDirectory `json:"national_emergency,omitempty"`
	SyncedAt          time.Time            `json:"synced_at"`
}

// PublicStatsResponse adalah payload response untuk GET /api/v1/stats/public.
type PublicStatsResponse struct {
	TotalTrackedNumbers   int            `json:"total_tracked_numbers"`
	DangerScamCount       int            `json:"danger_scam_count"`
	SpamCount             int            `json:"spam_count"`
	SuspiciousCount       int            `json:"suspicious_count"`
	EmergencyAgencies     int            `json:"emergency_agencies"`
	OfficialChannelsCount int            `json:"official_channels_count"`
	CategoryTotals        map[string]int `json:"category_totals"`
	UpdatedAt             time.Time      `json:"updated_at"`
}
