package controller

import (
	"context"
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

	var req model.AuthLoginRequest
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "Format JSON request tidak valid")
		return
	}

	normalizedPhone := helper.NormalizePhoneID(req.Phone)
	if len(normalizedPhone) < 9 {
		helper.WriteError(w, http.StatusBadRequest, "Nomor telepon tidak valid (gunakan format 08xx atau +628xx)")
		return
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = r.Header.Get("X-Device-Id")
	}
	if deviceID == "" {
		deviceID = "android-default-device"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, err := config.FindOrCreateUser(ctx, normalizedPhone, req.DisplayName, deviceID)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal memproses data pengguna: "+err.Error())
		return
	}

	token, exp, err := helper.GenerateToken(user.ID.Hex(), user.PhoneHash, user.DisplayName, user.Role, user.TrustWeight)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan token autentikasi")
		return
	}

	user.PhoneE164 = normalizedPhone
	resp := model.AuthLoginResponse{
		Token:     token,
		TokenType: "Bearer",
		ExpiresAt: exp,
		User:      user,
	}

	helper.WriteSuccess(w, http.StatusOK, "Autentikasi pengguna berhasil", resp)
}
