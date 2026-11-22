package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

// AuthLogin menangani POST /api/v1/auth/device untuk autentikasi perangkat dan registrasi otomatis pengguna CallTag.
func AuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan POST")
		return
	}

	if config.GoogleClientID() != "" || config.IsProdEnv() {
		helper.WriteError(w, http.StatusGone, "Gunakan masuk dengan Google")
		return
	}

	var req model.AuthLoginRequest
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteDecodeError(w, err)
		return
	}

	normalizedPhone := ""
	if strings.TrimSpace(req.Phone) != "" {
		normalizedPhone = helper.NormalizePhoneID(req.Phone)
		if len(normalizedPhone) < 9 {
			helper.WriteError(w, http.StatusBadRequest, "Nomor telepon tidak valid (gunakan format 08xx atau +628xx)")
			return
		}
	}

	deviceID := strings.TrimSpace(req.InstallID)
	if deviceID == "" {
		deviceID = strings.TrimSpace(req.DeviceID)
	}
	if deviceID == "" {
		deviceID = strings.TrimSpace(r.Header.Get("X-Device-Id"))
	}
	if deviceID == "" {
		if normalizedPhone == "" {
			helper.WriteError(w, http.StatusBadRequest, "install_id atau device_id wajib diisi")
			return
		}
		deviceID = "android-default-device"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, err := config.FindOrCreateUser(ctx, normalizedPhone, req.DisplayName, deviceID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal memproses data pengguna: "+err.Error())
		return
	}

	token, exp, err := helper.GenerateTokenWithVersion(user.ID.Hex(), user.PhoneHash, user.DisplayName, user.Role, user.TrustWeight, user.TokenVersion)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan token autentikasi")
		return
	}

	refreshToken, err := config.IssueRefreshToken(ctx, user.ID.Hex())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan refresh token")
		return
	}

	user.PhoneE164 = normalizedPhone
	resp := model.AuthLoginResponse{
		Token:        token,
		AccessToken:  token,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    exp,
		User:         user,
	}

	helper.WriteSuccess(w, http.StatusOK, "Autentikasi pengguna berhasil", resp)
}

// AuthRefresh menangani POST /api/v1/auth/refresh dengan rotasi token dan deteksi pemakaian ulang (T-204).
func AuthRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan POST")
		return
	}

	var req model.RefreshRequest
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteDecodeError(w, err)
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		helper.WriteError(w, http.StatusBadRequest, "refresh_token wajib diisi")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, newRefresh, err := config.RotateRefreshToken(ctx, req.RefreshToken)
	if err != nil {
		if errors.Is(err, config.ErrInvalidRefreshToken) || errors.Is(err, config.ErrUserInactive) {
			helper.WriteError(w, http.StatusUnauthorized, "Refresh token tidak valid, sudah kedaluwarsa, atau sesi telah dicabut")
			return
		}
		helper.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui sesi autentikasi: "+err.Error())
		return
	}

	token, exp, err := helper.GenerateTokenWithVersion(user.ID.Hex(), user.PhoneHash, user.DisplayName, user.Role, user.TrustWeight, user.TokenVersion)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan access token baru")
		return
	}

	resp := model.AuthLoginResponse{
		Token:        token,
		AccessToken:  token,
		RefreshToken: newRefresh,
		TokenType:    "Bearer",
		ExpiresAt:    exp,
		User:         user,
	}
	helper.WriteSuccess(w, http.StatusOK, "Token autentikasi berhasil diperbarui", resp)
}

// MeHandler menangani GET /api/v1/me (profil + sisa kuota) dan DELETE /api/v1/me (penghapusan akun UU PDP) untuk T-204.
func MeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET atau DELETE")
		return
	}

	claims, isAuth, err := helper.AuthenticateRequest(r)
	if err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "Token autentikasi tidak valid atau kedaluwarsa: "+err.Error())
		return
	}
	if !isAuth || claims == nil || strings.TrimSpace(claims.UserID) == "" || strings.EqualFold(claims.Role, "GUEST") {
		helper.WriteError(w, http.StatusUnauthorized, "Autentikasi pengguna diperlukan untuk mengakses profil")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if !config.IsUserActive(ctx, claims.UserID, claims.TokenVersion) {
		helper.WriteError(w, http.StatusUnauthorized, "Sesi tidak valid atau akun telah dihapus")
		return
	}

	if r.Method == http.MethodDelete {
		if err := config.RequestAccountDeletion(ctx, claims.UserID); err != nil {
			if errors.Is(err, config.ErrUserInactive) || errors.Is(err, config.ErrNotFound) {
				helper.WriteError(w, http.StatusUnauthorized, "Akun tidak ditemukan atau sudah dihapus")
				return
			}
			helper.WriteError(w, http.StatusInternalServerError, "Gagal memproses penghapusan akun: "+err.Error())
			return
		}
		helper.WriteSuccess(w, http.StatusAccepted, "Permintaan penghapusan akun diterima; identitas pelapor telah dianonimkan", map[string]interface{}{
			"user_id":    claims.UserID,
			"status":     "DELETION_REQUESTED",
			"anonymized": true,
		})
		return
	}

	user, err := config.GetUserByID(ctx, claims.UserID)
	if err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "Data pengguna tidak ditemukan")
		return
	}
	quota, err := config.GetUserQuotaRemaining(ctx, claims.UserID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menghitung sisa kuota pengguna: "+err.Error())
		return
	}

	resp := model.MeResponse{
		ID:             user.ID.Hex(),
		DisplayName:    user.DisplayName,
		Email:          user.Email,
		PhoneMask:      user.PhoneMask,
		Role:           user.Role,
		Status:         user.Status,
		TrustWeight:    user.TrustWeight,
		AccuracyStats:  user.AccuracyStats,
		QuotaRemaining: quota,
		CreatedAt:      user.CreatedAt,
	}
	helper.WriteSuccess(w, http.StatusOK, "Profil pengguna dan sisa kuota berhasil diambil", resp)
}
