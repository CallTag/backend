package helper

import (
	"testing"
	"time"
)

func TestStaffTOTP(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	now := time.Unix(59, 0)
	code, err := StaffTOTPCode(secret, now)
	if err != nil || code != "287082" {
		t.Fatalf("RFC6238 six digits: %q %v", code, err)
	}
	step, ok := VerifyStaffTOTP(secret, code, now, -1)
	if !ok || step != 1 {
		t.Fatal("valid code rejected")
	}
	if _, ok := VerifyStaffTOTP(secret, code, now, step); ok {
		t.Fatal("replay accepted")
	}
	if _, ok := VerifyStaffTOTP(secret, "000000", now, -1); ok {
		t.Fatal("bad code accepted")
	}
}
