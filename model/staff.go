package model

import "time"

// Staff credentials live separately from device registrations: device roles never grant staff access.
type Staff struct {
	ID           string `bson:"_id" json:"id"`
	Username     string `bson:"username" json:"username"`
	Role         string `bson:"role" json:"role"`
	PasswordHash string `bson:"password_hash" json:"-"`
	TOTPSecret   string `bson:"totp_secret" json:"-"`
	LastTOTPStep int64  `bson:"last_totp_step" json:"-"`
	Active       bool   `bson:"active" json:"-"`
}
type StaffSession struct {
	TokenHash string    `bson:"_id" json:"-"`
	StaffID   string    `bson:"staff_id" json:"-"`
	CSRFToken string    `bson:"csrf_token" json:"-"`
	ExpiresAt time.Time `bson:"expires_at" json:"expires_at"`
}
type StaffPrincipal struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CSRFToken string `json:"csrf_token"`
}
