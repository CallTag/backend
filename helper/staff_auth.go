package helper

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const StaffCookieName = "calltag_staff_session"

func StaffRandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func StaffTokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func StaffTOTPCode(secret string, now time.Time) (string, error) {
	return staffTOTPAtStep(secret, now.Unix()/30)
}
func staffTOTPAtStep(secret string, step int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(key) < 20 {
		return "", errors.New("TOTP secret requires at least 160 bits")
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 15
	n := (binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff) % 1000000
	return fmt.Sprintf("%06d", n), nil
}

// VerifyStaffTOTP accepts a 30-second drift window and returns the consumed timestep.
func VerifyStaffTOTP(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for _, d := range []int64{0, -1, 1} {
		step := now.Unix()/30 + d
		if step <= lastStep {
			continue
		}
		want, err := staffTOTPAtStep(secret, step)
		if err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
func StaffRoleAllowed(role, minRole string) bool {
	role = NormalizeStaffRole(role)
	minRole = NormalizeStaffRole(minRole)
	return role == "ADMIN" && (minRole == "ADMIN" || minRole == "MOD") || role == "MOD" && minRole == "MOD"
}
func NormalizeStaffRole(role string) string {
	role = strings.ToUpper(strings.TrimSpace(role))
	if role == "MODERATOR" {
		return "MOD"
	}
	return role
}
