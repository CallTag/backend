package config

import (
	"context"
	"errors"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const ColStaff = "staff_accounts"
const ColStaffSessions = "staff_sessions"

var ErrStaffAuth = errors.New("staff authentication rejected")
var staffMu sync.Mutex
var staffAccounts = map[string]model.Staff{}
var staffSessions = map[string]model.StaffSession{}

func ResetStaffForTest() {
	staffMu.Lock()
	defer staffMu.Unlock()
	staffAccounts = map[string]model.Staff{}
	staffSessions = map[string]model.StaffSession{}
}
func ResetStaffMemoryForTest() { ResetStaffForTest() }
func staffMemoryAllowed() bool { return !IsProdEnv() && os.Getenv("STAFF_ALLOW_MEMORY") == "true" }
func staffStorageReady() error {
	if MongoConnected && MongoDB != nil {
		return nil
	}
	if staffMemoryAllowed() {
		return nil
	}
	return errors.New("staff storage unavailable; development memory requires STAFF_ALLOW_MEMORY=true")
}
func EnsureStaffIndexes(ctx context.Context) error {
	if !MongoConnected || MongoDB == nil {
		return nil
	}
	indexes := MongoDB.Collection(ColStaff).Indexes()
	if _, err := indexes.CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(
			bson.M{"username": bson.M{"$exists": true}},
		),
	}); err != nil {
		return err
	}
	// The old non-sparse index treated a missing email as a unique value.
	if _, err := indexes.DropOne(ctx, "email_1"); err != nil {
		var commandErr mongo.CommandError
		if !errors.As(err, &commandErr) || !commandErr.HasErrorCode(27) {
			return err
		}
	}
	_, err := MongoDB.Collection(ColStaffSessions).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)})
	return err
}

var staffUsernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

func NormalizeStaffUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// SetLegacyStaffUsername attaches a username to a legacy account in place.
func SetLegacyStaffUsername(ctx context.Context, email, username string) error {
	username = NormalizeStaffUsername(username)
	if !staffUsernamePattern.MatchString(username) {
		return errors.New("invalid username")
	}
	if !MongoConnected || MongoDB == nil {
		return errors.New("MongoDB required")
	}
	result, err := MongoDB.Collection(ColStaff).UpdateOne(ctx,
		bson.M{"email": strings.ToLower(strings.TrimSpace(email)), "username": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"username": username}},
	)
	if err != nil {
		return err
	}
	if result.ModifiedCount != 1 {
		return errors.New("legacy account not found or already assigned a username")
	}
	return nil
}

func CreateStaff(ctx context.Context, username, password, role, secret string) error {
	if err := staffStorageReady(); err != nil {
		return err
	}
	username = NormalizeStaffUsername(username)
	if !staffUsernamePattern.MatchString(username) {
		return errors.New("invalid username")
	}
	role = helper.NormalizeStaffRole(role)
	if role != "ADMIN" && role != "MOD" {
		return errors.New("invalid role")
	}
	if len(password) < 8 || len(password) > 72 {
		return errors.New("password must contain 8 to 72 bytes")
	}
	if _, err := helper.StaffTOTPCode(secret, time.Now()); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	id, err := helper.StaffRandomToken()
	if err != nil {
		return err
	}
	staff := model.Staff{ID: id, Username: username, Role: role, PasswordHash: string(hash), TOTPSecret: strings.ToUpper(strings.TrimSpace(secret)), LastTOTPStep: -1, Active: true}
	if MongoConnected && MongoDB != nil {
		_, err := MongoDB.Collection(ColStaff).InsertOne(ctx, staff)
		return err
	}
	staffMu.Lock()
	defer staffMu.Unlock()
	if _, ok := staffAccounts[username]; ok {
		return errors.New("staff already exists")
	}
	staffAccounts[username] = staff
	return nil
}
func FindStaffByUsername(ctx context.Context, username string) (model.Staff, error) {
	if err := staffStorageReady(); err != nil {
		return model.Staff{}, err
	}
	username = NormalizeStaffUsername(username)
	if MongoConnected && MongoDB != nil {
		var s model.Staff
		err := MongoDB.Collection(ColStaff).FindOne(ctx, bson.M{"username": username, "active": true}).Decode(&s)
		return s, err
	}
	staffMu.Lock()
	defer staffMu.Unlock()
	s, ok := staffAccounts[username]
	if !ok || !s.Active {
		return model.Staff{}, ErrStaffAuth
	}
	return s, nil
}

var staffDummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing-only"), 12)

func AuthenticateStaff(ctx context.Context, username, password, code string, now time.Time) (model.Staff, error) {
	s, err := FindStaffByUsername(ctx, username)
	hash := s.PasswordHash
	if err != nil {
		hash = string(staffDummyHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil || passwordErr != nil {
		return model.Staff{}, ErrStaffAuth
	}
	step, ok := helper.VerifyStaffTOTP(s.TOTPSecret, code, now, s.LastTOTPStep)
	if !ok {
		return model.Staff{}, ErrStaffAuth
	}
	if MongoConnected && MongoDB != nil {
		res, err := MongoDB.Collection(ColStaff).UpdateOne(ctx, bson.M{"_id": s.ID, "active": true, "last_totp_step": bson.M{"$lt": step}}, bson.M{"$set": bson.M{"last_totp_step": step}})
		if err != nil {
			return model.Staff{}, err
		}
		if res.ModifiedCount != 1 {
			return model.Staff{}, ErrStaffAuth
		}
	} else {
		staffMu.Lock()
		current, ok := staffAccounts[s.Username]
		if !ok || !current.Active || current.LastTOTPStep >= step {
			staffMu.Unlock()
			return model.Staff{}, ErrStaffAuth
		}
		current.LastTOTPStep = step
		staffAccounts[s.Username] = current
		staffMu.Unlock()
	}
	s.LastTOTPStep = step
	return s, nil
}
func IssueStaffSession(ctx context.Context, staff model.Staff, now time.Time) (string, model.StaffSession, error) {
	if err := staffStorageReady(); err != nil {
		return "", model.StaffSession{}, err
	}
	raw, err := helper.StaffRandomToken()
	if err != nil {
		return "", model.StaffSession{}, err
	}
	csrf, err := helper.StaffRandomToken()
	if err != nil {
		return "", model.StaffSession{}, err
	}
	s := model.StaffSession{TokenHash: helper.StaffTokenHash(raw), StaffID: staff.ID, CSRFToken: csrf, ExpiresAt: now.Add(8 * time.Hour)}
	if MongoConnected && MongoDB != nil {
		_, err = MongoDB.Collection(ColStaffSessions).InsertOne(ctx, s)
	} else {
		staffMu.Lock()
		staffSessions[s.TokenHash] = s
		staffMu.Unlock()
	}
	return raw, s, err
}
func ResolveStaffSession(ctx context.Context, raw string, now time.Time) (model.StaffPrincipal, error) {
	if err := staffStorageReady(); err != nil {
		return model.StaffPrincipal{}, err
	}
	if len(raw) != 43 {
		return model.StaffPrincipal{}, ErrStaffAuth
	}
	hash := helper.StaffTokenHash(raw)
	var s model.StaffSession
	var staff model.Staff
	if MongoConnected && MongoDB != nil {
		if err := MongoDB.Collection(ColStaffSessions).FindOne(ctx, bson.M{"_id": hash, "expires_at": bson.M{"$gt": now}}).Decode(&s); err != nil {
			return model.StaffPrincipal{}, ErrStaffAuth
		}
		if err := MongoDB.Collection(ColStaff).FindOne(ctx, bson.M{"_id": s.StaffID, "active": true}).Decode(&staff); err != nil {
			return model.StaffPrincipal{}, ErrStaffAuth
		}
	} else {
		staffMu.Lock()
		defer staffMu.Unlock()
		var ok bool
		s, ok = staffSessions[hash]
		if !ok || !now.Before(s.ExpiresAt) {
			delete(staffSessions, hash)
			return model.StaffPrincipal{}, ErrStaffAuth
		}
		for _, candidate := range staffAccounts {
			if candidate.ID == s.StaffID && candidate.Active {
				staff = candidate
				break
			}
		}
		if staff.ID == "" {
			return model.StaffPrincipal{}, ErrStaffAuth
		}
	}
	return model.StaffPrincipal{ID: staff.ID, Username: staff.Username, Role: staff.Role, CSRFToken: s.CSRFToken}, nil
}
func RevokeStaffSession(ctx context.Context, raw string) error {
	if err := staffStorageReady(); err != nil {
		return err
	}
	hash := helper.StaffTokenHash(raw)
	if MongoConnected && MongoDB != nil {
		_, err := MongoDB.Collection(ColStaffSessions).DeleteOne(ctx, bson.M{"_id": hash})
		return err
	}
	staffMu.Lock()
	defer staffMu.Unlock()
	delete(staffSessions, hash)
	return nil
}
