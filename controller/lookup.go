package controller

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

// LookupPhone menangani GET /api/v1/phones/{e164}.
func LookupPhone(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	if _, _, err := helper.AuthenticateRequest(r); err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "Token autentikasi tidak valid atau kedaluwarsa: "+err.Error())
		return
	}

	rawPhone := helper.ExtractPathParam(r.URL.Path, "/api/v1/phones/")
	if rawPhone == "" {
		rawPhone = r.URL.Query().Get("phone")
	}

	if decoded, err := url.PathUnescape(rawPhone); err == nil {
		rawPhone = decoded
	}
	rawPhone = strings.TrimSuffix(rawPhone, "/reports")

	phoneE164 := helper.NormalizePhoneID(rawPhone)
	if phoneE164 == "" || len(strings.TrimPrefix(phoneE164, "+")) < 3 {
		helper.WriteError(w, http.StatusBadRequest, "Parameter nomor telepon wajib diisi (contoh: /api/v1/phones/+6281299887766)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pt, reports, err := config.GetPhoneLookup(ctx, phoneE164)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal memeriksa reputasi nomor: "+err.Error())
		return
	}

	verified, err := config.GetActiveVerifiedNumber(ctx, phoneE164, time.Now().UTC())
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal memeriksa verifikasi nomor: "+err.Error())
		return
	}
	if verified != nil && pt == nil {
		pt = &model.PhoneTag{PhoneE164: phoneE164, Carrier: helper.DetectIndonesianCarrier(phoneE164), ReputationLevel: model.ReputationClean, Tags: []model.TagCount{}, CategoryBreakdown: map[string]int{}}
	}

	if pt == nil {
		cleanResp := model.LookupResponse{
			PhoneE164:         phoneE164,
			Carrier:           helper.DetectIndonesianCarrier(phoneE164),
			Found:             false,
			ReputationLevel:   model.ReputationClean,
			RiskScore:         0.0,
			SpamScore:         0.0,
			IsDanger:          false,
			Tags:              []model.TagCount{},
			CategoryBreakdown: map[string]int{},
			UniqueReporters:   0,
			TotalReports:      0,
		}
		helper.WriteSuccess(w, http.StatusOK, "Nomor belum pernah dilaporkan (Status: CLEAN)", cleanResp)
		return
	}

	var lastRep *time.Time
	if !pt.LastReportedAt.IsZero() {
		t := pt.LastReportedAt
		lastRep = &t
	}

	score := pt.RiskScore
	if score == 0 {
		score = pt.SpamScore
	}

	resp := model.LookupResponse{
		PhoneE164:             pt.PhoneE164,
		Carrier:               helper.DetectIndonesianCarrier(phoneE164),
		Found:                 true,
		ReputationLevel:       pt.ReputationLevel,
		RiskScore:             score,
		SpamScore:             score,
		IsDanger:              pt.IsDanger,
		IsOfficialBlacklisted: pt.IsOfficialBlacklisted,
		IsWhitelistedVerified: pt.IsWhitelistedVerified,
		PrimaryCategory:       pt.PrimaryCategory,
		PrimaryCategoryLabel:  pt.PrimaryCategoryLabel,
		TopCommunityLabel:     pt.TopCommunityLabel,
		Tags:                  pt.Tags,
		CategoryBreakdown:     pt.CategoryBreakdown,
		UniqueReporters:       pt.UniqueReporters,
		TotalReports:          pt.TotalReports,
		RecentReports:         reports,
		LastReportedAt:        lastRep,
	}

	// Status verifikasi selalu mengikuti bukti aktif, bukan flag agregat yang dapat usang.
	resp.IsWhitelistedVerified = verified != nil
	if verified != nil {
		resp.VerifiedLabel = verified.Kind
		resp.VerifiedSourceURL = verified.SourceURL
		resp.VerifiedAt = &verified.VerifiedAt
		resp.TopCommunityLabel = verified.DisplayLabel
		resp.ReputationLevel = model.ReputationClean
		resp.RiskScore, resp.SpamScore, resp.IsDanger = 0, 0, false
	}

	helper.WriteSuccess(w, http.StatusOK, "Informasi reputasi nomor berhasil diambil", resp)
}

// GetPhoneReports menangani GET /api/v1/phones/{e164}/reports sesuai PRD Bagian 5.3.
func GetPhoneReports(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	if _, _, err := helper.AuthenticateRequest(r); err != nil {
		helper.WriteError(w, http.StatusUnauthorized, "Token autentikasi tidak valid atau kedaluwarsa: "+err.Error())
		return
	}

	rawPhone := helper.ExtractPathParam(r.URL.Path, "/api/v1/phones/")
	if decoded, err := url.PathUnescape(rawPhone); err == nil {
		rawPhone = decoded
	}
	rawPhone = strings.TrimSuffix(rawPhone, "/reports")

	phoneE164 := helper.NormalizePhoneID(rawPhone)
	if phoneE164 == "" {
		helper.WriteError(w, http.StatusBadRequest, "Parameter nomor telepon tidak valid")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pt, reports, err := config.GetPhoneLookup(ctx, phoneE164)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar laporan nomor: "+err.Error())
		return
	}

	if reports == nil {
		reports = []model.SpamReport{}
	}

	if pt == nil {
		helper.WriteSuccess(w, http.StatusOK, "Belum ada laporan komunitas untuk nomor ini", model.PhoneReportsResponse{
			PhoneE164:         phoneE164,
			ReputationLevel:   model.ReputationClean,
			RiskScore:         0.0,
			SpamScore:         0.0,
			UniqueReporters:   0,
			TotalReports:      0,
			CategoryBreakdown: map[string]int{},
			Reports:           reports,
			RecentReports:     reports,
		})
		return
	}

	score := pt.RiskScore
	if score == 0 {
		score = pt.SpamScore
	}

	helper.WriteSuccess(w, http.StatusOK, "Daftar laporan komunitas berhasil diambil", model.PhoneReportsResponse{
		PhoneE164:         pt.PhoneE164,
		ReputationLevel:   pt.ReputationLevel,
		RiskScore:         score,
		SpamScore:         score,
		TopCommunityLabel: pt.TopCommunityLabel,
		UniqueReporters:   pt.UniqueReporters,
		TotalReports:      pt.TotalReports,
		CategoryBreakdown: pt.CategoryBreakdown,
		Reports:           reports,
		RecentReports:     reports,
	})
}
