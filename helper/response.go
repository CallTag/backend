package helper

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/CallTag/backend/model"
)

// MaxBodyBytes adalah batas maksimal ukuran payload JSON request (16 KB) sesuai spesifikasi Tahap 4 (T-203).
const MaxBodyBytes = 16 * 1024

// ErrBodyTooLarge dikembalikan saat ukuran request body melebihi MaxBodyBytes.
var ErrBodyTooLarge = errors.New("ukuran payload request melebihi batas maksimal 16 KB")

// SetCORS mengatur header CORS agar API dapat diakses oleh aplikasi mobile, apidocs, dan calltag.github.io.
func SetCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Device-Id, Idempotency-Key, X-Tracking-Token")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}

// WriteJSON menulis respons JSON terstandarisasi sesuai format gocroot.
func WriteJSON(w http.ResponseWriter, statusCode int, success bool, message string, data interface{}) {
	SetCORS(w)
	w.WriteHeader(statusCode)
	resp := model.APIResponse{
		Success:   success,
		Code:      statusCode,
		Message:   message,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// WriteSuccess menulis respons sukses (200/201).
func WriteSuccess(w http.ResponseWriter, statusCode int, message string, data interface{}) {
	WriteJSON(w, statusCode, true, message, data)
}

// WriteError menulis respons gagal (4xx/5xx).
func WriteError(w http.ResponseWriter, statusCode int, message string) {
	WriteJSON(w, statusCode, false, message, nil)
}

// DecodeJSON membaca dan mem-parsing body JSON dari request dengan batas ukuran 16 KB (T-203).
func DecodeJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	limited := io.LimitReader(r.Body, MaxBodyBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if len(raw) > MaxBodyBytes {
		return ErrBodyTooLarge
	}
	return json.Unmarshal(raw, dst)
}

// WriteDecodeError menulis respons 413 jika payload terlalu besar atau 400 untuk JSON rusak (T-203).
func WriteDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrBodyTooLarge) {
		WriteError(w, http.StatusRequestEntityTooLarge, "Ukuran payload melebihi batas maksimal 16 KB")
		return
	}
	WriteError(w, http.StatusBadRequest, "Format JSON request tidak valid")
}

// ExtractPathParam mengambil segmen terakhir setelah prefix path tertentu.
// Contoh: ExtractPathParam("/api/v1/phones/+62812345678", "/api/v1/phones/") -> "+62812345678"
func ExtractPathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	val := strings.TrimPrefix(path, prefix)
	val = strings.Trim(val, "/")
	return val
}
