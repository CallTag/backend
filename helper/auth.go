package helper

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

// TokenClaims menyimpan payload JWT untuk autentikasi pengguna CallTag.
type TokenClaims struct {
	UserID      string  `json:"uid"`
	PhoneHash   string  `json:"ph"`
	DisplayName string  `json:"name"`
	Role        string  `json:"role"`
	TrustWeight float64 `json:"tw"`
	IssuedAt    int64   `json:"iat"`
	ExpiresAt   int64   `json:"exp"`
}

func getSecretKey() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "calltag-indonesia-gocroot-secret-key-2026"
	}
	return []byte(secret)
}

// HashIdentity menghasilkan hash SHA-256 dengan salt sesuai aturan privasi UU PDP di PRD.
func HashIdentity(raw string) string {
	salt := os.Getenv("HASH_SALT")
	if salt == "" {
		salt = "calltag-uupdp-salt-id"
	}
	h := sha256.New()
	h.Write([]byte(salt + ":" + strings.TrimSpace(raw)))
	return "sha256_" + hex.EncodeToString(h.Sum(nil))
}

// GenerateToken membuat JWT (HS256) tanpa ketergantungan library eksternal yang berat.
func GenerateToken(userID, phoneHash, displayName, role string, trustWeight float64) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(7 * 24 * time.Hour)

	headerJSON := `{"alg":"HS256","typ":"JWT"}`
	claims := TokenClaims{
		UserID:      userID,
		PhoneHash:   phoneHash,
		DisplayName: displayName,
		Role:        role,
		TrustWeight: trustWeight,
		IssuedAt:    now.Unix(),
		ExpiresAt:   exp.Unix(),
	}

	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}

	headerEnc := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	claimsEnc := base64.RawURLEncoding.EncodeToString(claimsBytes)
	signingInput := headerEnc + "." + claimsEnc

	mac := hmac.New(sha256.New, getSecretKey())
	mac.Write([]byte(signingInput))
	sigEnc := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + sigEnc, exp, nil
}

// VerifyToken memvalidasi token JWT HS256 dan mengembalikan TokenClaims.
func VerifyToken(tokenStr string) (*TokenClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, errors.New("format token tidak valid")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, getSecretKey())
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

// ExtractClaimsFromRequest mengambil klaim dari header Authorization: Bearer <token>.
// Jika tidak ada header, mengembalikan klaim tamu (guest) dengan trust_weight standar agar pelaporan publik/demo tetap mudah diuji.
func ExtractClaimsFromRequest(r *http.Request) (*TokenClaims, bool) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		tokenStr := strings.TrimSpace(authHeader[7:])
		if claims, err := VerifyToken(tokenStr); err == nil {
			return claims, true
		}
	}

	deviceID := strings.TrimSpace(r.Header.Get("X-Device-Id"))
	if deviceID == "" {
		deviceID = r.RemoteAddr
	}
	guestHash := HashIdentity(deviceID)
	return &TokenClaims{
		UserID:      "guest_" + guestHash[7:19],
		PhoneHash:   guestHash,
		DisplayName: "Warga Komunitas",
		Role:        "USER",
		TrustWeight: 0.8,
	}, false
}
