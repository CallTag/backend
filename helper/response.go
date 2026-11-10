package helper

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/CallTag/backend/model"
)

// SetCORS mengatur header CORS agar API dapat diakses oleh aplikasi mobile, apidocs, dan calltag.github.io.
func SetCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Device-Id")
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

// DecodeJSON membaca dan mem-parsing body JSON dari request.
func DecodeJSON(r *http.Request, dst interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(dst)
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
