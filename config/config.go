package config

import (
	"errors"
	"os"
	"strings"
)

// Nama-nama koleksi MongoDB sesuai CallTag/PRD.md dan desain Tahap 3.
const (
	ColUsers              = "users"
	ColPhoneTags          = "phone_tags"
	ColSpamReports        = "spam_reports"
	ColEmergencyDirectory = "emergency_directory"
	ColOfficialChannels   = "official_channels"
	ColModerationCases    = "moderation_cases"
	ColSyncChanges        = "sync_changes"
	ColSyncCounters       = "sync_counters"
	ColRefreshTokens      = "refresh_tokens"
)

// GetAppEnv mengambil mode lingkungan aplikasi yang dipilih operator.
func GetAppEnv() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
}

// IsProdEnv mengembalikan true apabila backend berjalan dalam mode produksi (prod/production).
func IsProdEnv() bool {
	env := GetAppEnv()
	return env == "prod" || env == "production"
}

// ValidateConfig mewajibkan mode eksplisit dan konfigurasi keamanan produksi.
func ValidateConfig() error {
	switch GetAppEnv() {
	case "dev":
		return nil
	case "prod", "production":
	default:
		return errors.New("APP_ENV wajib diisi dengan dev, prod, atau production")
	}

	if strings.TrimSpace(GetMongoURI()) == "" {
		return errors.New("konfigurasi produksi ditolak: MONGOSTRING wajib diisi pada APP_ENV=prod")
	}
	if len(strings.TrimSpace(os.Getenv("JWT_SECRET"))) < 32 {
		return errors.New("konfigurasi produksi ditolak: JWT_SECRET wajib minimal 32 karakter pada APP_ENV=prod")
	}
	if len(strings.TrimSpace(os.Getenv("HASH_SALT"))) < 16 {
		return errors.New("konfigurasi produksi ditolak: HASH_SALT wajib minimal 16 karakter pada APP_ENV=prod")
	}
	return nil
}

// GetMongoURI mengambil connection string MongoDB dari environment variable MONGOSTRING (konvensi gocroot) atau MONGODB_URI.
func GetMongoURI() string {
	uri := strings.TrimSpace(os.Getenv("MONGOSTRING"))
	if uri == "" {
		uri = strings.TrimSpace(os.Getenv("MONGODB_URI"))
	}
	return uri
}

// GetDBName mengambil nama database MongoDB (default: "calltag_db").
func GetDBName() string {
	name := os.Getenv("DB_NAME")
	if name == "" {
		name = "calltag_db"
	}
	return name
}

// GetPort mengambil port HTTP server (default: "8080").
func GetPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return port
}
