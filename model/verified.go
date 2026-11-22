package model

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// VerifiedNumber menyimpan kontak publik terkurasi dengan bukti sumber yang memiliki masa berlaku.
type VerifiedNumber struct {
	ID           primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`
	PhoneE164    string             `json:"phone_e164" bson:"phone_e164"`
	Kind         string             `json:"kind" bson:"kind"`
	DisplayLabel string             `json:"display_label" bson:"display_label"`
	SourceURL    string             `json:"source_url" bson:"source_url"`
	VerifiedBy   string             `json:"verified_by" bson:"verified_by"`
	VerifiedAt   time.Time          `json:"verified_at" bson:"verified_at"`
	ExpiresAt    time.Time          `json:"expires_at" bson:"expires_at"`
	Status       string             `json:"status" bson:"status"`
}
