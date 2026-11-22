package controller

import (
	"context"
	"crypto/hmac"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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

	// T-101 & T-103: Wajib token autentikasi User yang valid; Guest & token rusak ditolak 401.
	claims, isAuth, err := helper.AuthenticateRequest(r)
	if err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "Token autentikasi tidak valid atau kedaluwarsa: "+err.Error())
		return
	}
	if !isAuth || claims == nil || strings.TrimSpace(claims.UserID) == "" || strings.EqualFold(claims.Role, "GUEST") {
		helper.WriteError(w, http.StatusUnauthorized, "Autentikasi pengguna diperlukan untuk mengirim laporan spam")
		return
	}

	var req model.ReportRequest
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteDecodeError(w, err)
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
		helper.WriteError(w, http.StatusBadRequest, "Kategori tidak valid. Gunakan salah satu: PINJOL_DC, APK_SNIFFING, IMPERSONATION, JUDOL, SOCIAL_ENG, TELEMARKETING, INFO_TAG")
		return
	}

	tagName := strings.TrimSpace(req.CustomTag)
	if tagName == "" {
		tagName = strings.TrimSpace(req.TagName)
	}
	if utf8.RuneCountInString(tagName) > 40 {
		helper.WriteError(w, http.StatusBadRequest, "Panjang custom_tag maksimal 40 karakter")
		return
	}
	if tagName == "" {
		tagName = catMeta.Label
		if utf8.RuneCountInString(tagName) > 40 {
			tagName = string([]rune(tagName)[:40])
		}
	}

	tagName = helper.SanitizeCommentPII(tagName)
	rawComment := strings.TrimSpace(req.Comment)
	if utf8.RuneCountInString(rawComment) > 500 {
		helper.WriteError(w, http.StatusBadRequest, "Panjang komentar maksimal 500 karakter")
		return
	}
	sanitizedComment := helper.SanitizeCommentPII(rawComment)

	callType := strings.TrimSpace(req.CallType)
	if callType == "" {
		callType = "CELLULAR_CALL"
	}

	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idemKey) > 128 {
		helper.WriteError(w, http.StatusBadRequest, "Header Idempotency-Key maksimal 128 karakter")
		return
	}

	// T-103: Identitas pelapor murni berasal dari JWT claims.UserID (mengabaikan X-Device-Id)
	reportDoc := model.SpamReport{
		Source:                "APP",
		PhoneE164:             phoneE164,
		ReporterUserID:        claims.UserID,
		IdempotencyKey:        idemKey,
		CategoryCode:          catMeta.Code,
		CustomTag:             tagName,
		TagName:               tagName,
		Comment:               sanitizedComment,
		CallType:              callType,
		ReporterTrustSnapshot: claims.TrustWeight,
		AppliedWeight:         catMeta.Weight,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if !config.IsUserActive(ctx, claims.UserID, claims.TokenVersion) {
		helper.WriteError(w, http.StatusUnauthorized, "Sesi tidak valid atau akun telah dihapus")
		return
	}

	savedReport, updatedTag, created, err := config.CreateReportWithIdempotency(ctx, reportDoc, idemKey)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrQuotaExceeded):
			w.Header().Set("Retry-After", "300")
			helper.WriteError(w, http.StatusTooManyRequests, "Batas kuota pelaporan terlampaui (maksimal 3 laporan per 5 menit atau 10 laporan per 24 jam)")
		case errors.Is(err, config.ErrIdempotencyMismatch):
			helper.WriteError(w, http.StatusConflict, err.Error())
		default:
			helper.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan dan mengagregasi laporan: "+err.Error())
		}
		return
	}

	log.Printf("[controller/report] Laporan diproses target=%s created=%v\n", helper.MaskPhone(phoneE164), created)

	recommendedHub, _ := config.GetOfficialChannelsByCategory(ctx, catMeta.Code)

	resp := model.ReportResponse{
		ReportID:               savedReport.ID.Hex(),
		PhoneTag:               updatedTag,
		RecommendedOfficialHub: recommendedHub,
	}

	if created {
		helper.WriteSuccess(w, http.StatusCreated, "Laporan berhasil disimpan dan skor reputasi nomor telah diperbarui", resp)
		return
	}
	helper.WriteSuccess(w, http.StatusOK, "Laporan sudah tercatat sebelumnya (idempotent)", resp)
}

// SubmitDispute menangani POST /api/v1/disputes bagi pemilik nomor atau tamu yang mengajukan sanggahan (T-105).
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

	claims, isAuth, err := helper.AuthenticateRequest(r)
	if err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "Token autentikasi tidak valid atau kedaluwarsa: "+err.Error())
		return
	}

	var req model.DisputeRequest
	if err := helper.DecodeJSON(r, &req); err != nil {
		helper.WriteDecodeError(w, err)
		return
	}

	phoneE164 := helper.NormalizePhoneID(req.Phone)
	ownerName := strings.TrimSpace(req.OwnerName)
	reason := strings.TrimSpace(req.Reason)
	contactRef := strings.TrimSpace(req.ContactRef)

	if len(phoneE164) < 8 || ownerName == "" || reason == "" {
		helper.WriteError(w, http.StatusBadRequest, "Nomor telepon, nama pemilik, dan alasan sanggahan wajib diisi")
		return
	}
	if utf8.RuneCountInString(ownerName) > 120 || utf8.RuneCountInString(reason) > 1000 || utf8.RuneCountInString(contactRef) > 120 {
		helper.WriteError(w, http.StatusBadRequest, "Panjang data sanggahan melebihi batas (owner_name <= 120, reason <= 1000, contact_ref <= 120)")
		return
	}

	var submitterID string
	if isAuth && claims != nil {
		submitterID = "user:" + claims.UserID
	} else {
		guestSeed := strings.TrimSpace(r.Header.Get("X-Device-Id"))
		if guestSeed == "" {
			host, _, splitErr := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
			if splitErr == nil && host != "" {
				guestSeed = host
			} else {
				guestSeed = strings.TrimSpace(r.RemoteAddr)
			}
		}
		submitterID = "guest:" + helper.HashIdentity(guestSeed)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	createdCase, trackingToken, err := config.CreateDisputeCase(ctx, req, submitterID)
	if err != nil {
		switch {
		case errors.Is(err, config.ErrDuplicateDispute):
			helper.WriteError(w, http.StatusConflict, err.Error())
		case errors.Is(err, config.ErrQuotaExceeded):
			w.Header().Set("Retry-After", "3600")
			helper.WriteError(w, http.StatusTooManyRequests, "Batas pengajuan sanggahan terlampaui (maksimal 3 sanggahan per 24 jam)")
		default:
			helper.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengajuan sanggahan: "+err.Error())
		}
		return
	}

	log.Printf("[controller/report] Sanggahan tersimpan dispute_id=%s target=%s\n", createdCase.DisputeID, helper.MaskPhone(createdCase.PhoneE164))

	resp := model.DisputeResponse{
		DisputeID:     createdCase.DisputeID,
		PhoneE164:     createdCase.PhoneE164,
		OwnerName:     createdCase.OwnerName,
		Reason:        createdCase.Reason,
		Status:        createdCase.Status,
		TrackingToken: trackingToken,
		SubmittedAt:   createdCase.SubmittedAt,
		UpdatedAt:     createdCase.UpdatedAt,
	}

	helper.WriteSuccess(w, http.StatusCreated, "Pengajuan sanggahan (dispute) berhasil disimpan dengan status UNDER_REVIEW", resp)
}

// GetDispute menangani GET /api/v1/disputes/{id} untuk memantau status sanggahan yang tersimpan (T-105).
func GetDispute(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	claims, isAuth, err := helper.AuthenticateRequest(r)
	if err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "Token autentikasi tidak valid atau kedaluwarsa: "+err.Error())
		return
	}

	disputeID := helper.ExtractPathParam(r.URL.Path, "/api/v1/disputes/")
	if disputeID == "" || strings.Contains(disputeID, "/") {
		helper.WriteError(w, http.StatusBadRequest, "Parameter dispute_id tidak valid")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	doc, err := config.GetDisputeCaseByID(ctx, disputeID)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			helper.WriteError(w, http.StatusNotFound, "Sanggahan tidak ditemukan")
			return
		}
		helper.WriteError(w, http.StatusInternalServerError, "Gagal mengambil status sanggahan: "+err.Error())
		return
	}

	isOwnerOrStaff := isAuth && claims != nil && (doc.SubmitterID == "user:"+claims.UserID || claims.Role == "MODERATOR" || claims.Role == "ADMIN")
	trackingHeader := strings.TrimSpace(r.Header.Get("X-Tracking-Token"))
	tokenAuthorized := false
	if trackingHeader != "" {
		actualHash := helper.HashValue("tracking:" + trackingHeader)
		if !hmac.Equal([]byte(actualHash), []byte(doc.TrackingTokenHash)) {
			helper.WriteError(w, http.StatusForbidden, "X-Tracking-Token sanggahan tidak valid")
			return
		}
		tokenAuthorized = true
	}

	resp := model.DisputeResponse{
		DisputeID:   doc.DisputeID,
		PhoneE164:   doc.PhoneE164,
		OwnerName:   doc.OwnerName,
		Status:      doc.Status,
		SubmittedAt: doc.SubmittedAt,
		UpdatedAt:   doc.UpdatedAt,
	}
	if isOwnerOrStaff || tokenAuthorized {
		resp.Reason = doc.Reason
		resp.ResolutionNotes = doc.ResolutionNotes
	}

	helper.WriteSuccess(w, http.StatusOK, "Status sanggahan berhasil diambil", resp)
}
