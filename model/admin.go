package model

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// AuditLog is append-only. Private document snapshots and authentication material
// are deliberately omitted: hashes permit comparison without exposing evidence.
type AuditLog struct {
	ID         primitive.ObjectID `json:"id" bson:"_id"`
	Actor      string             `json:"actor" bson:"actor"`
	Action     string             `json:"action" bson:"action"`
	Target     string             `json:"target" bson:"target"`
	Reason     string             `json:"reason" bson:"reason"`
	BeforeHash string             `json:"before_hash" bson:"before_hash"`
	AfterHash  string             `json:"after_hash" bson:"after_hash"`
	CreatedAt  time.Time          `json:"created_at" bson:"created_at"`
}

type AdminDecision struct {
	Actor      string               `json:"-" bson:"actor"`
	Action     string               `json:"-" bson:"action"`
	Reason     string               `json:"-" bson:"reason"`
	Version    int                  `json:"-" bson:"version"`
	BeforeHash string               `json:"-" bson:"before_hash"`
	ReportIDs  []primitive.ObjectID `json:"-" bson:"report_ids"`
	At         time.Time            `json:"-" bson:"at"`
	Done       bool                 `json:"-" bson:"done"`
}

type AdminCaseSummary struct {
	QueueCount      int `json:"queue_count"`
	OverdueDisputes int `json:"overdue_disputes"`
	OverdueReports  int `json:"overdue_reports"`
	Reports24h      int `json:"reports_24h"`
}
type AdminCaseQueue struct {
	Items   []ModerationCase `json:"items"`
	Summary AdminCaseSummary `json:"summary"`
}
type AdminCaseDetail struct {
	Case    ModerationCase `json:"case"`
	Phone   *PhoneTag      `json:"phone"`
	Reports []SpamReport   `json:"reports"`
}
