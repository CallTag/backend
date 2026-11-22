package helper

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var warnDevSecretOnce sync.Once

// TokenClaims menyimpan payload JWT untuk autentikasi pengguna CallTag.
type TokenClaims struct {
	UserID       string  `json:"uid"`
	PhoneHash    string  `json:"ph"`
	DisplayName  string  `json:"name"`
	Role         string  `json:"role"`
	TrustWeight  float64 `json:"tw"`
	TokenVersion int     `json:"ver,omitempty"`
	IssuedAt     int64   `json:"iat"`
	ExpiresAt    int64   `json:"exp"`
}

func isProdRuntime() bool {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	return env == "prod" || env == "production"
}

func getJWTSecretStrict() ([]byte, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if isProdRuntime() {
		if len(secret) < 32 {
			return nil, errors.New("JWT_SECRET wajib diset minimal 32 karakter pada mode produksi")
		}
		return []byte(secret), nil
	}
	if secret == "" {
		warnDevSecretOnce.Do(func() {
			log.Println("[helper/auth] PERINGATAN: JWT_SECRET belum diset, memakai secret pengembangan lokal (dilarang di produksi).")
		})
		secret = "calltag-indonesia-gocroot-secret-key-2026"
	}
	return []byte(secret), nil
}

func getHashSaltStrict() (string, error) {
	salt := strings.TrimSpace(os.Getenv("HASH_SALT"))
	if isProdRuntime() {
		if len(salt) < 16 {
			return "", errors.New("HASH_SALT wajib diset minimal 16 karakter pada mode produksi")
		}
		return salt, nil
	}
	if salt == "" {
		salt = "calltag-uupdp-salt-id"
	}
	return salt, nil
}

// HashValue menghasilkan hex digest SHA-256 sepanjang 64 karakter dengan HASH_SALT.
func HashValue(raw string) string {
	salt, err := getHashSaltStrict()
	if err != nil {
		// Pada mode produksi yang belum tervalidasi, tolak pemakaian salt statis.
		h := sha256.New()
		h.Write([]byte(time.Now().UTC().Format(time.RFC3339Nano) + ":" + strings.TrimSpace(raw)))
		return hex.EncodeToString(h.Sum(nil))
	}
	h := sha256.New()
	h.Write([]byte(salt + ":" + strings.TrimSpace(raw)))
	return hex.EncodeToString(h.Sum(nil))
}

// HashIdentity menghasilkan hash SHA-256 dengan salt sesuai aturan privasi UU PDP di PRD.
func HashIdentity(raw string) string {
	return "sha256_" + HashValue(raw)
}

// GenerateToken membuat JWT (HS256) dengan versi token default 1.
func GenerateToken(userID, phoneHash, displayName, role string, trustWeight float64) (string, time.Time, error) {
	return GenerateTokenWithVersion(userID, phoneHash, displayName, role, trustWeight, 1)
}

// GenerateTokenWithVersion membuat JWT (HS256) menyertakan token_version untuk validasi pencabutan sesi (T-204).
func GenerateTokenWithVersion(userID, phoneHash, displayName, role string, trustWeight float64, tokenVersion int) (string, time.Time, error) {
	secret, err := getJWTSecretStrict()
	if err != nil {
		return "", time.Time{}, err
	}
	if tokenVersion <= 0 {
		tokenVersion = 1
	}

	now := time.Now().UTC()
	exp := now.Add(7 * 24 * time.Hour)

	headerJSON := `{"alg":"HS256","typ":"JWT"}`
	claims := TokenClaims{
		UserID:       userID,
		PhoneHash:    phoneHash,
		DisplayName:  displayName,
		Role:         role,
		TrustWeight:  trustWeight,
		TokenVersion: tokenVersion,
		IssuedAt:     now.Unix(),
		ExpiresAt:    exp.Unix(),
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}

	headerEnc := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := headerEnc + "." + claimsEnc

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	sigEnc := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + sigEnc, exp, nil
}

// VerifyToken memvalidasi token JWT HS256 dan mengembalikan TokenClaims.
func VerifyToken(tokenStr string) (*TokenClaims, error) {
	secret, err := getJWTSecretStrict()
	if err != nil {
		return nil, err
	}

	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, errors.New("format token tidak valid")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errors.New("tanda tangan token tidak valid")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("payload token rusak")
	}

	var claims TokenClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, errors.New("gagal membaca klaim token")
	}

	if time.Now().UTC().Unix() > claims.ExpiresAt {
		return nil, errors.New("token telah kedaluwarsa")
	}

	return &claims, nil
}

// AuthenticateRequest memverifikasi header Authorization secara ketat (T-101):
// - Jika header Authorization tidak ada -> kembalikan klaim GUEST, isAuthenticated=false, err=nil.
// - Jika header Authorization ada tetapi format/tanda tangan/masa berlaku tidak valid -> kembalikan error (401), bukan GUEST.
func AuthenticateRequest(r *http.Request) (*TokenClaims, bool, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return &TokenClaims{
			DisplayName: "Tamu",
			Role:        "GUEST",
		}, false, nil
	}

	if len(authHeader) < 7 || !strings.EqualFold(authHeader[:7], "bearer ") {
		return nil, false, errors.New("format header Authorization wajib Bearer <token>")
	}

	tokenStr := strings.TrimSpace(authHeader[7:])
	if tokenStr == "" {
		return nil, false, errors.New("token Bearer kosong")
	}

	claims, err := VerifyToken(tokenStr)
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(claims.UserID) == "" {
		return nil, false, errors.New("klaim identitas pengguna pada token tidak valid")
	}
	return claims, true, nil
}

// ExtractClaimsFromRequest adalah pembungkus kompatibel bagi pemanggil yang hanya memerlukan info auth opsional.
func ExtractClaimsFromRequest(r *http.Request) (*TokenClaims, bool) {
	claims, isAuth, err := AuthenticateRequest(r)
	if err != nil || !isAuth {
		return &TokenClaims{
			DisplayName: "Tamu",
			Role:        "GUEST",
		}, false
	}
	return claims, true
}
