package controller

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CallTag/backend/config"

	"github.com/CallTag/backend/internal/dataset"
	"github.com/CallTag/backend/model"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func AdminCuration(w http.ResponseWriter, r *http.Request) {
	principal, ok := RequireStaff(w, r, "ADMIN")
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/"), "/"), "/")
	if len(parts) > 2 || len(parts) == 0 {
		adminCurationError(w, 404, "Rute tidak ditemukan")
		return
	}
	kind := parts[0]
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}
	if r.Method == http.MethodGet {
		rows, err := config.ListAdminCuration(r.Context(), kind)
		if err != nil {
			adminCurationError(w, 500, "Gagal mengambil data")
			return
		}
		staffJSON(w, 200, "Data kurasi", rows)
		return
	}
	switch r.Method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		adminCurationError(w, 405, "Metode tidak diizinkan")
		return
	}
	if (r.Method == "POST" && key != "") || (r.Method != "POST" && key == "") {
		adminCurationError(w, 400, "Identitas path tidak valid")
		return
	}
	var fields map[string]interface{}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024*1024))
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		adminCurationError(w, 400, "JSON tidak valid")
		return
	}
	if decoder.Decode(new(interface{})) != io.EOF {
		adminCurationError(w, 400, "Hanya satu objek JSON diizinkan")
		return
	}
	reason, _ := fields["reason"].(string)
	row, err := config.MutateAdminCuration(r.Context(), kind, key, r.Method, principal.ID, reason, fields)
	if err != nil {
		status := 400
		if errors.Is(err, config.ErrCurationConflict) {
			status = 409
		}
		if errors.Is(err, config.ErrCurationNotFound) {
			status = 404
		}
		adminCurationError(w, status, err.Error())
		return
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	staffJSON(w, status, "Kurasi tersimpan dan diaudit", row)
}

// AdminImport validates the complete file before any write. The accepted file
// formats share the CLI importer for verified/scam/tag_context.
func AdminImport(w http.ResponseWriter, r *http.Request) {
	principal, ok := RequireStaff(w, r, "ADMIN")
	if !ok {
		return
	}
	if r.Method != "POST" {
		adminCurationError(w, 405, "Gunakan POST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 33*1024*1024)
	if err := r.ParseMultipartForm(2 * 1024 * 1024); err != nil {
		adminCurationError(w, 400, "Multipart tidak valid atau terlalu besar")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	source := strings.TrimSpace(r.FormValue("source"))
	if reason == "" || len(reason) > 2000 || source == "" {
		adminCurationError(w, 400, "source dan reason wajib")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		adminCurationError(w, 400, "file wajib")
		return
	}
	defer file.Close()
	kind := r.URL.Query().Get("kind")
	extension := strings.ToLower(filepath.Ext(header.Filename))
	dry := r.URL.Query().Get("dry_run") == "true"
	if kind == "emergency" {
		rows, err := parseEmergencyBatch(io.LimitReader(file, 32*1024*1024), extension, source, principal.ID)
		if err != nil {
			adminCurationError(w, 400, err.Error())
			return
		}
		result := model.AdminBatchResult{Validated: len(rows), DryRun: dry}
		if !dry {
			existing, e := config.ListAdminCuration(r.Context(), "emergency")
			if e != nil {
				adminCurationError(w, 500, "Gagal membaca direktori")
				return
			}
			existingCodes := make(map[string]bool, len(existing))
			for _, v := range existing {
				if code, ok := v["code"].(string); ok && code != "" {
					existingCodes[code] = true
				}
			}
			for _, row := range rows {
				// A prior identical import may be retried; each applied row retains its audit.
				data := adminJSONMap(row)
				data["reason"] = reason
				method := "POST"
				if existingCodes[row.Code] {
					method = "PUT"
				}
				if _, err = config.MutateAdminCuration(r.Context(), "emergency", row.Code, method, principal.ID, reason, data); err != nil {
					adminCurationError(w, 500, "Impor berhenti; baris tersimpan tetap diaudit: "+err.Error())
					return
				}
				existingCodes[row.Code] = true
				result.Written++
			}
		}
		staffJSON(w, 200, "Impor direktori", result)
		return
	}
	rows, err := dataset.Parse(file, extension, kind, source)
	if err != nil {
		adminCurationError(w, 400, err.Error())
		return
	}
	result, err := config.ImportAdminDataset(r.Context(), rows, kind, principal.ID, reason, dry)
	if err != nil {
		adminCurationError(w, 500, "Impor gagal: "+err.Error())
		return
	}
	staffJSON(w, 200, "Impor dataset", result)
}
func adminJSONMap(v interface{}) map[string]interface{} {
	b, _ := json.Marshal(v)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	return m
}
func parseEmergencyBatch(reader io.Reader, ext, source, actor string) ([]model.EmergencyDirectory, error) {
	rows := []model.EmergencyDirectory{}
	switch ext {
	case ".json":
		d := json.NewDecoder(reader)
		d.DisallowUnknownFields()
		if err := d.Decode(&rows); err != nil {
			return nil, err
		}
		if d.Decode(new(interface{})) != io.EOF {
			return nil, errors.New("trailing JSON")
		}
	case ".csv":
		r := csv.NewReader(reader)
		head, err := r.Read()
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for i, k := range head {
			k = strings.TrimPrefix(strings.TrimSpace(k), "\ufeff")
			if seen[k] {
				return nil, errors.New("duplicate CSV header")
			}
			seen[k] = true
			head[i] = k
		}
		for {
			cells, e := r.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, e
			}
			m := map[string]interface{}{}
			for i, k := range head {
				switch k {
				case "contacts", "capabilities", "location":
					var v interface{}
					if cells[i] != "" {
						if err = json.Unmarshal([]byte(cells[i]), &v); err != nil {
							return nil, fmt.Errorf("invalid JSON in %s CSV cell", k)
						}
						m[k] = v
					}
				case "is_24_hours":
					if cells[i] != "true" && cells[i] != "false" && cells[i] != "" {
						return nil, errors.New("invalid is_24_hours")
					}
					m[k] = cells[i] == "true"
				default:
					m[k] = cells[i]
				}
			}
			b, _ := json.Marshal(m)
			var row model.EmergencyDirectory
			if err = json.Unmarshal(b, &row); err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	default:
		return nil, errors.New("file must be CSV or JSON")
	}
	if len(rows) == 0 || len(rows) > 10000 {
		return nil, errors.New("emergency batch requires 1–10000 records")
	}
	seen := map[string]bool{}
	for i := range rows {
		v := &rows[i]
		if v.SourceURL == "" {
			v.SourceURL = source
		}
		v.VerifiedBy = actor
		if e := config.ValidateEmergencyCuration(*v); e != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, e)
		}
		if seen[v.Code] {
			return nil, errors.New("duplicate emergency code")
		}
		seen[v.Code] = true
		v.ID = [12]byte{}
	}
	return rows, nil
}
func AdminExport(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireStaff(w, r, "ADMIN"); !ok {
		return
	}
	if r.Method != "GET" {
		adminCurationError(w, 405, "Gunakan GET")
		return
	}
	dataset := r.URL.Query().Get("dataset")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		adminCurationError(w, 400, "format harus json atau csv")
		return
	}
	rows, err := config.ExportAdminDataset(r.Context(), dataset)
	if err != nil {
		adminCurationError(w, 400, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, dataset, format))
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	writer := csv.NewWriter(w)
	defer writer.Flush()
	keys := exportCSVHeaders(dataset, rows)
	_ = writer.Write(keys)
	for _, row := range rows {
		cells := make([]string, len(keys))
		for i, k := range keys {
			var value string
			switch v := row[k].(type) {
			case nil:
				value = ""
			case time.Time:
				value = v.UTC().Format(time.RFC3339)
			case string:
				value = v
			default:
				value = fmt.Sprint(v)
			}
			cells[i] = safeCSVCell(value)
		}
		_ = writer.Write(cells)
	}
}
func exportCSVHeaders(dataset string, rows []map[string]interface{}) []string {
	if len(rows) > 0 {
		keys := make([]string, 0, len(rows[0]))
		for k := range rows[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	switch dataset {
	case "verified_numbers":
		return []string{"display_label", "expires_at", "kind", "phone_e164", "source_url", "status", "verified_at"}
	case "phone_tags":
		return []string{"phone_e164", "primary_category", "reputation_level", "risk_score", "total_reports", "unique_reporters", "updated_at"}
	case "moderation_cases":
		return []string{"dispute_id", "phone_e164", "status", "submitted_at", "type", "updated_at", "version"}
	default:
		return []string{}
	}
}
func safeCSVCell(s string) string {
	trim := strings.TrimLeft(s, " \t\r\n")
	if strings.ContainsAny(s[:len(s)-len(trim)], "\t\r\n") || (trim != "" && strings.ContainsRune("=+-@", rune(trim[0]))) {
		return "'" + s
	}
	return s
}

func adminCurationError(w http.ResponseWriter, status int, message string) {
	staffJSON(w, status, message, nil)
}
