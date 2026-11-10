package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

// GetNationalEmergency menangani GET /api/v1/emergency/national sesuai PRD Bagian 5.3.
func GetNationalEmergency(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	national, _, err := config.QueryEmergency(ctx, "", "", r.URL.Query().Get("type"))
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal mengambil nomor darurat nasional: "+err.Error())
		return
	}
	if national == nil {
		national = []model.EmergencyDirectory{}
	}

	helper.WriteSuccess(w, http.StatusOK, "Daftar 7 nomor darurat nasional berhasil diambil", model.EmergencyResponse{
		NationalNumbers: national,
		LocalDirectory:  []model.EmergencyDirectory{},
	})
}

// GetEmergencyDirectory menangani GET /api/v1/emergency/local dengan filter query parameter city, province, dan type.
func GetEmergencyDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	q := r.URL.Query()
	province := q.Get("province")
	city := q.Get("city")
	agencyType := q.Get("type")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	national, local, err := config.QueryEmergency(ctx, province, city, agencyType)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal mengambil direktori darurat: "+err.Error())
		return
	}

	officialChannels, _ := config.GetOfficialChannelsByCategory(ctx, "")

	if national == nil {
		national = []model.EmergencyDirectory{}
	}
	if local == nil {
		local = []model.EmergencyDirectory{}
	}

	resp := model.EmergencyResponse{
		FilterProvince:   province,
		FilterCity:       city,
		FilterAgencyType: agencyType,
		NationalNumbers:  national,
		LocalDirectory:   local,
		OfficialChannels: officialChannels,
	}

	helper.WriteSuccess(w, http.StatusOK, "Direktori nomor darurat nasional dan lokal berhasil diambil", resp)
}

// GetOfficialChannels menangani GET /api/v1/official-channels untuk Pusat Panduan Lapor Resmi.
func GetOfficialChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		helper.SetCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, http.StatusMethodNotAllowed, "Metode HTTP tidak diizinkan, gunakan GET")
		return
	}

	cat := model.CategoryCode(r.URL.Query().Get("category"))
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	channels, err := config.GetOfficialChannelsByCategory(ctx, cat)
	if err != nil {
		helper.WriteError(w, http.StatusInternalServerError, "Gagal mengambil kanal pelaporan resmi: "+err.Error())
		return
	}
	helper.WriteSuccess(w, http.StatusOK, "Daftar kanal pelaporan resmi pemerintah berhasil diambil", channels)
}
