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
	CategoryInfoTag       CategoryCode = "INFO_TAG"
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
	GoogleSub        string             `json:"-" bson:"google_sub,omitempty"`
	Email            string             `json:"email,omitempty" bson:"email,omitempty"`
	PhoneE164        string             `json:"phone_e164,omitempty" bson:"-"`
	PhoneMask        string             `json:"phone_mask,omitempty" bson:"phone_mask,omitempty"`
	PhoneHash        string             `json:"phone_hash,omitempty" bson:"phone_hash,omitempty"`
	DeviceIDHash     string             `json:"device_id_hash" bson:"device_id_hash"`
	DisplayName      string             `json:"display_name" bson:"display_name"`
	Role             string             `json:"role" bson:"role"`
	TrustWeight      float64            `json:"trust_weight" bson:"trust_weight"`
	AccuracyStats    AccuracyStats      `json:"accuracy_stats" bson:"accuracy_stats"`
	DailyReportCount int                `json:"daily_report_count" bson:"daily_report_count"`
	LastReportAt     time.Time          `json:"last_report_at,omitempty" bson:"last_report_at,omitempty"`
	TokenVersion     int                `json:"token_version" bson:"token_version"`
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
	ID                       primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	PhoneE164                string             `json:"phone_e164" bson:"phone_e164"`
	CountryCode              string             `json:"country_code" bson:"country_code"`
	Carrier                  string             `json:"carrier" bson:"carrier"`
	ReputationLevel          ReputationLevel    `json:"reputation_level" bson:"reputation_level"`
	RiskScore                float64            `json:"risk_score" bson:"risk_score"`
	SpamScore                float64            `json:"spam_score" bson:"-"`
	PrimaryCategory          CategoryCode       `json:"primary_category" bson:"primary_category"`
	PrimaryCategoryLabel     string             `json:"primary_category_label" bson:"primary_category_label"`
	IsDanger                 bool               `json:"is_danger" bson:"is_danger"`
	ModeratorConfirmedDanger bool               `json:"moderator_confirmed_danger" bson:"moderator_confirmed_danger"`
	PendingDangerReview      bool               `json:"pending_danger_review" bson:"pending_danger_review"`
	TopCommunityLabel        string             `json:"top_community_label" bson:"top_community_label"`
	Tags                     []TagCount         `json:"tags" bson:"tags"`
	CategoryBreakdown        map[string]int     `json:"category_breakdown" bson:"category_breakdown"`
	UniqueReporters          int                `json:"unique_reporters" bson:"unique_reporters"`
	TotalReports             int                `json:"total_reports" bson:"total_reports"`
	IsOfficialBlacklisted    bool               `json:"is_official_blacklisted" bson:"is_official_blacklisted"`
	IsWhitelistedVerified    bool               `json:"is_whitelisted_verified" bson:"is_whitelisted_verified"`
	SyncVersion              int64              `json:"sync_version" bson:"sync_version"`
	FirstReportedAt          time.Time          `json:"first_reported_at,omitempty" bson:"first_reported_at,omitempty"`
	LastReportedAt           time.Time          `json:"last_reported_at,omitempty" bson:"last_reported_at,omitempty"`
	LastActivityAt           time.Time          `json:"last_activity_at,omitempty" bson:"last_activity_at,omitempty"`
	ExpiresAt                time.Time          `json:"expires_at,omitempty" bson:"expires_at,omitempty"`
	UpdatedAt                time.Time          `json:"updated_at" bson:"updated_at"`
}

// SpamReport merepresentasikan dokumen laporan individual pada koleksi "spam_reports".
type SpamReport struct {
	Source                string             `json:"source,omitempty" bson:"source,omitempty"`
	ID                    primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	PhoneE164             string             `json:"phone_e164" bson:"phone_e164"`
	ReporterUserID        string             `json:"reporter_user_id,omitempty" bson:"reporter_user_id"`
	IdempotencyKey        string             `json:"idempotency_key,omitempty" bson:"idempotency_key,omitempty"`
	WindowKey             int64              `json:"-" bson:"window_key"`
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
	SourceURL    string                  `json:"source_url" bson:"source_url"`
	VerifiedBy   string                  `json:"verified_by" bson:"verified_by"`
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
	SourceURL                 string               `json:"source_url" bson:"source_url"`
	VerifiedBy                string               `json:"verified_by" bson:"verified_by"`
	VerifiedAt                time.Time            `json:"verified_at" bson:"verified_at"`
	SyncVersion               int64                `json:"sync_version" bson:"sync_version"`
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
	InstallID   string `json:"install_id"`
}

// AuthLoginResponse adalah payload response untuk POST /api/v1/auth/device dan POST /api/v1/auth/refresh.
type AuthLoginResponse struct {
	Token        string    `json:"token"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         User      `json:"user"`
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
	VerifiedLabel         string          `json:"verified_label,omitempty"`
	VerifiedSourceURL     string          `json:"verified_source_url,omitempty"`
	VerifiedAt            *time.Time      `json:"verified_at,omitempty"`
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

// DisputeRequest adalah payload request untuk POST /api/v1/disputes.
type DisputeRequest struct {
	Phone      string `json:"phone"`
	OwnerName  string `json:"owner_name"`
	Reason     string `json:"reason"`
	ContactRef string `json:"contact_ref,omitempty"`
}

// ModerationCase merepresentasikan dokumen sanggahan dan kasus moderasi pada koleksi "moderation_cases".
type ModerationCase struct {
	Decision          *AdminDecision     `json:"-" bson:"admin_decision,omitempty"`
	ID                primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	DisputeID         string             `json:"dispute_id" bson:"dispute_id"`
	Type              string             `json:"type" bson:"type"`
	PhoneE164         string             `json:"phone_e164" bson:"phone_e164"`
	OwnerName         string             `json:"owner_name" bson:"owner_name"`
	Reason            string             `json:"reason" bson:"reason"`
	ContactRef        string             `json:"contact_ref,omitempty" bson:"contact_ref,omitempty"`
	SubmitterID       string             `json:"-" bson:"submitter_id"`
	Status            string             `json:"status" bson:"status"`
	TrackingTokenHash string             `json:"-" bson:"tracking_token_hash,omitempty"`
	ResolutionNotes   string             `json:"resolution_notes,omitempty" bson:"resolution_notes,omitempty"`
	Version           int                `json:"version" bson:"version"`
	SubmittedAt       time.Time          `json:"submitted_at" bson:"submitted_at"`
	UpdatedAt         time.Time          `json:"updated_at" bson:"updated_at"`
}

// DisputeResponse adalah payload response untuk POST /api/v1/disputes dan GET /api/v1/disputes/{id}.
type DisputeResponse struct {
	DisputeID       string    `json:"dispute_id"`
	PhoneE164       string    `json:"phone_e164"`
	OwnerName       string    `json:"owner_name"`
	Reason          string    `json:"reason,omitempty"`
	Status          string    `json:"status"`
	TrackingToken   string    `json:"tracking_token,omitempty"`
	ResolutionNotes string    `json:"resolution_notes,omitempty"`
	SubmittedAt     time.Time `json:"submitted_at"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// RefreshTokenDoc merepresentasikan sesi refresh token ter-hash pada koleksi "refresh_tokens" (T-204).
type RefreshTokenDoc struct {
	ID        primitive.ObjectID `json:"-" bson:"_id,omitempty"`
	TokenHash string             `json:"-" bson:"token_hash"`
	UserID    string             `json:"user_id" bson:"user_id"`
	ExpiresAt time.Time          `json:"expires_at" bson:"expires_at"`
	RevokedAt *time.Time         `json:"revoked_at,omitempty" bson:"revoked_at,omitempty"`
	CreatedAt time.Time          `json:"created_at" bson:"created_at"`
}

// RefreshRequest adalah payload request untuk POST /api/v1/auth/refresh (T-204).
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// QuotaRemaining menyimpan sisa kuota pelaporan dan sanggahan pengguna (T-204).
type QuotaRemaining struct {
	Reports5mRemaining   int `json:"reports_5m_remaining"`
	Reports24hRemaining  int `json:"reports_24h_remaining"`
	Disputes24hRemaining int `json:"disputes_24h_remaining"`
}

// MeResponse adalah payload response untuk GET /api/v1/me (T-204).
type MeResponse struct {
	ID             string         `json:"id"`
	DisplayName    string         `json:"display_name"`
	Email          string         `json:"email,omitempty"`
	PhoneMask      string         `json:"phone_mask,omitempty"`
	Role           string         `json:"role"`
	Status         string         `json:"status"`
	TrustWeight    float64        `json:"trust_weight"`
	AccuracyStats  AccuracyStats  `json:"accuracy_stats"`
	QuotaRemaining QuotaRemaining `json:"quota_remaining"`
	CreatedAt      time.Time      `json:"created_at"`
}

// SyncChange merepresentasikan entri jurnal perubahan berurutan pada koleksi "sync_changes" (T-201).
type SyncChange struct {
	ID         primitive.ObjectID     `json:"-" bson:"_id,omitempty"`
	Seq        int64                  `json:"seq" bson:"seq"`
	Generation int                    `json:"generation" bson:"generation"`
	Op         string                 `json:"op" bson:"op"`
	Kind       string                 `json:"kind" bson:"kind"`
	Key        string                 `json:"key" bson:"key"`
	Payload    map[string]interface{} `json:"payload,omitempty" bson:"payload,omitempty"`
	At         time.Time              `json:"at" bson:"at"`
}

// SyncChangesResponse adalah payload response untuk GET /api/v1/sync/changes (T-201).
type SyncChangesResponse struct {
	Generation int          `json:"generation"`
	Count      int          `json:"count"`
	Items      []SyncChange `json:"items"`
	NextCursor string       `json:"next_cursor"`
	HasMore    bool         `json:"has_more"`
	SyncedAt   time.Time    `json:"synced_at"`
}
