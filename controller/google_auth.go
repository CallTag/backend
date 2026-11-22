package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/auth/credentials/idtoken"
	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

// AuthGoogle menukar bukti identitas Google dengan sesi CallTag.
func AuthGoogle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Gunakan POST")
		return
	}
	clientID := config.GoogleClientID()
	if clientID == "" {
		helper.WriteError(w, http.StatusServiceUnavailable, "Login Google belum dikonfigurasi")
		return
	}

	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteDecodeError(w, err)
		return
	}
	if req.IDToken == "" || len(req.IDToken) > 8192 {
		helper.WriteError(w, http.StatusBadRequest, "Token Google wajib diisi dan harus valid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	payload, err := idtoken.Validate(ctx, req.IDToken, clientID)
	if err != nil || (payload.Issuer != "https://accounts.google.com" && payload.Issuer != "accounts.google.com") ||
		payload.Subject == "" || len(payload.Subject) > 255 {
		helper.WriteError(w, http.StatusUnauthorized, "Identitas Google tidak dapat diverifikasi")
		return
	}

	email, _ := payload.Claims["email"].(string)
	emailVerified, _ := payload.Claims["email_verified"].(bool)
	if !emailVerified || strings.TrimSpace(email) == "" || len(email) > 320 {
		helper.WriteError(w, http.StatusUnauthorized, "Email Google belum terverifikasi")
		return
	}
	name, _ := payload.Claims["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Pengguna CallTag"
	}
	if runes := []rune(name); len(runes) > 120 {
		name = string(runes[:120])
	}
	user, err := config.FindOrCreateGoogleUser(ctx, payload.Subject, name, email)
	if err != nil {
		helper.WriteError(w, http.StatusServiceUnavailable, "Akun Google belum dapat dimuat")
		return
	}
	token, exp, err := helper.GenerateTokenWithVersion(user.ID.Hex(), "", user.DisplayName, user.Role, user.TrustWeight, user.TokenVersion)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan sesi CallTag")
		return
	}
	refreshToken, err := config.IssueRefreshToken(ctx, user.ID.Hex())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan sesi CallTag")
		return
	}
	helper.WriteSuccess(w, http.StatusOK, "Masuk dengan Google berhasil", model.AuthLoginResponse{
		Token: token, AccessToken: token, RefreshToken: refreshToken, TokenType: "Bearer", ExpiresAt: exp, User: user,
	})
}
