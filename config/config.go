package config

import "os"

// Nama-nama koleksi MongoDB sesuai CallTag/PRD.md.
const (
	ColUsers              = "users"
	ColPhoneTags          = "phone_tags"
	ColSpamReports        = "spam_reports"
	ColEmergencyDirectory = "emergency_directory"
	ColOfficialChannels   = "official_channels"
)

// GetMongoURI mengambil connection string MongoDB dari environment variable MONGOSTRING (konvensi gocroot) atau MONGODB_URI.
func GetMongoURI() string {
	uri := os.Getenv("MONGOSTRING")
	if uri == "" {
		uri = os.Getenv("MONGODB_URI")
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
