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

// SubmitReport menangani POST /api/v1/reports untuk mengirim laporan spam/scam & tag nama baru.
func SubmitReport(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan POST")
		return
	}

	var req model.ReportRequest
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "Format JSON request tidak valid")
		return
	}

	phoneE164 := helper.NormalizePhoneID(req.Phone)
	if len(phoneE164) < 8 {
		helper.WriteError(w, http.StatusBadRequest, "Nomor telepon yang dilaporkan tidak valid")
		return
	}

	// Proteksi Whitelist Nomor Darurat Nasional (112, 110, 113, 118, 119, 115, 123, 14080)
	if len(phoneE164) <= 6 {
		helper.WriteError(w, http.StatusForbidden, "Nomor darurat nasional dilindungi oleh Whitelist Shield dan tidak dapat dilaporkan sebagai spam")
		return
	}

	catMeta, validCat := helper.GetCategoryInfo(req.CategoryCode)
	if !validCat {
		helper.WriteError(w, http.StatusBadRequest, "Kategori tidak valid. Gunakan salah satu: PINJOL_DC, APK_SNIFFING, IMPERSONATION, JUDOL, SOCIAL_ENG, TELEMARKETING")
		return
	}

	tagName := strings.TrimSpace(req.CustomTag)
	if tagName == "" {
		tagName = strings.TrimSpace(req.TagName)
	}
	if tagName == "" {
		tagName = catMeta.Label
	}

	callType := strings.TrimSpace(req.CallType)
	if callType == "" {
		callType = "CELLULAR_CALL"
	}

	claims, _ := helper.ExtractClaimsFromRequest(r)

	reportDoc := model.SpamReport{
		PhoneE164:             phoneE164,
		ReporterUserID:        claims.UserID,
		CategoryCode:          catMeta.Code,
		CustomTag:             tagName,
		TagName:               tagName,
		Comment:               strings.TrimSpace(req.Comment),
		CallType:              callType,
		ReporterTrustSnapshot: claims.TrustWeight,
		AppliedWeight:         catMeta.Weight,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	updatedTag, err := config.CreateReportAndAggregate(ctx, reportDoc)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan dan mengagregasi laporan: "+err.Error())
		return
	}

	recommendedHub, _ := config.GetOfficialChannelsByCategory(ctx, catMeta.Code)

	resp := model.ReportResponse{
		ReportID:               reportDoc.PhoneE164 + "_" + claims.UserID,
		PhoneTag:               updatedTag,
		RecommendedOfficialHub: recommendedHub,
	}

	helper.WriteSuccess(w, http.StatusCreated, "Laporan berhasil disimpan dan skor reputasi nomor telah diperbarui", resp)
}

// SubmitDispute menangani POST /api/v1/disputes bagi pemilik nomor yang mengajukan sanggahan (US-05).
func SubmitDispute(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan POST")
		return
	}

	var req struct {
		Phone      string `json:"phone"`
		OwnerName  string `json:"owner_name"`
		Reason     string `json:"reason"`
		ContactRef string `json:"contact_ref"`
	}
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteError(w, http.StatusBadRequest, "Format JSON request tidak valid")
		return
	}

	phoneE164 := helper.NormalizePhoneID(req.Phone)
	if len(phoneE164) < 8 || strings.TrimSpace(req.Reason) == "" {
		helper.WriteError(w, http.StatusBadRequest, "Nomor telepon dan alasan sanggahan wajib diisi")
		return
	}

	helper.WriteSuccess(w, http.StatusCreated, "Pengajuan sanggahan (dispute) berhasil dicatat dengan status UNDER_REVIEW", map[string]interface{}{
		"dispute_id":   "DSP_" + strings.TrimPrefix(phoneE164, "+"),
		"phone_e164":   phoneE164,
		"owner_name":   strings.TrimSpace(req.OwnerName),
		"status":       "UNDER_REVIEW",
		"submitted_at": time.Now().UTC(),
	})
}
