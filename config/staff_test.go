package config

import (
	"context"
	"github.com/CallTag/backend/helper"
	"testing"
	"time"
)

func TestStaffCredentialsSessionAndReplay(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	ResetStaffMemoryForTest()
	ctx := context.Background()
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	const password = "Ab3!fG7x"
	if err := CreateStaff(ctx, " Admin ", password[:7], "ADMIN", secret); err == nil {
		t.Fatal("7-byte password accepted")
	}
	if err := CreateStaff(ctx, " Admin ", password, "ADMIN", secret); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	code, _ := helper.StaffTOTPCode(secret, now)
	staff, err := AuthenticateStaff(ctx, "admin", password, code, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AuthenticateStaff(ctx, staff.Username, password, code, now); err == nil {
		t.Fatal("replay accepted")
	}
	raw, session, err := IssueStaffSession(ctx, staff, now)
	if err != nil {
		t.Fatal(err)
	}
	if session.TokenHash == raw {
		t.Fatal("raw token stored")
	}
	if principal, err := ResolveStaffSession(ctx, raw, now); err != nil || principal.Username != "admin" {
		t.Fatalf("session principal = %+v, error = %v", principal, err)
	}
	if _, err := ResolveStaffSession(ctx, raw, now.Add(9*time.Hour)); err == nil {
		t.Fatal("expired session accepted")
	}
	if err := RevokeStaffSession(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveStaffSession(ctx, raw, now); err == nil {
		t.Fatal("revoked session accepted")
	}
}
func TestStaffMemoryFailClosed(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("STAFF_ALLOW_MEMORY", "true")
	if _, err := FindStaffByUsername(context.Background(), "nobody"); err == nil {
		t.Fatal("production memory accepted")
	}
}
