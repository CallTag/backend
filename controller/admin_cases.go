package controller

import (
	"errors"
	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"net/http"
	"strconv"
	"strings"
)

func AdminCases(w http.ResponseWriter, r *http.Request) {
	staff, ok := RequireStaff(w, r, "MODERATOR")
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/cases")
	id = strings.Trim(id, "/")
	id = strings.TrimSuffix(id, "/decision")
	if strings.Contains(id, "/") {
		helper.WriteError(w, 400, "Invalid case ID")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if id != "" {
			out, err := config.GetAdminCase(r.Context(), id)
			if err != nil {
				adminCaseError(w, err)
				return
			}
			out.Reports = reportsForStaff(out.Reports)
			helper.WriteSuccess(w, 200, "Case detail", out)
			return
		}
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 500 {
				helper.WriteError(w, 400, "Limit must be 1..500")
				return
			}
			limit = n
		}
		out, err := config.ListAdminCases(r.Context(), strings.ToUpper(r.URL.Query().Get("status")), strings.ToUpper(r.URL.Query().Get("type")), r.URL.Query().Get("q"), limit)
		if err != nil {
			adminCaseError(w, err)
			return
		}
		helper.WriteSuccess(w, 200, "Moderation queue", out)
	case http.MethodPatch:
		var req struct {
			CaseID  string `json:"case_id"`
			Version int    `json:"version"`
			Action  string `json:"action"`
			Outcome string `json:"outcome"`
			Reason  string `json:"reason"`
		}
		if err := helper.DecodeJSON(r, &req); err != nil {
			helper.WriteDecodeError(w, err)
			return
		}
		if id == "" {
			id = req.CaseID
		} else if req.CaseID != "" && req.CaseID != id {
			helper.WriteError(w, 400, "Case IDs differ")
			return
		}
		if req.Action == "" {
			req.Action = req.Outcome
		}
		switch req.Action {
		case "ACCEPTED":
			req.Action = "ACCEPT"
		case "CONFIRMED":
			req.Action = "CONFIRM_DANGER"
		case "REJECTED":
			req.Action = "REJECT"
		}
		out, err := config.DecideAdminCase(r.Context(), id, req.Version, req.Action, req.Reason, staff.ID)
		if err != nil {
			adminCaseError(w, err)
			return
		}
		helper.WriteSuccess(w, 200, "Moderation decision recorded", out)
	default:
		helper.WriteError(w, 405, "Use GET or PATCH")
	}
}
func adminCaseError(w http.ResponseWriter, err error) {
	code := 500
	if errors.Is(err, config.ErrCaseConflict) {
		code = 409
	}
	if errors.Is(err, config.ErrNotFound) {
		code = 404
	}
	if errors.Is(err, config.ErrInvalidDecision) {
		code = 400
	}
	helper.WriteError(w, code, http.StatusText(code))
}
func AdminPhoneProfile(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireStaff(w, r, "MODERATOR"); !ok {
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, 405, "Use GET")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/phones/")
	if p == r.URL.Path {
		p = strings.TrimPrefix(r.URL.Path, "/api/v1/admin/numbers/")
	}
	if strings.Contains(p, "/") {
		helper.WriteError(w, 400, "Invalid phone")
		return
	}
	phone, reports, err := config.GetAdminPhone(r.Context(), helper.NormalizePhoneID(p))
	if err != nil {
		adminCaseError(w, err)
		return
	}
	cases, audits, err := config.GetAdminPhoneHistory(r.Context(), helper.NormalizePhoneID(p))
	if err != nil {
		adminCaseError(w, err)
		return
	}
	helper.WriteSuccess(w, 200, "Phone profile", map[string]interface{}{"phone": phone, "reports": reportsForStaff(reports), "cases": cases, "audit": audits})
}

// Keep moderation evidence while withholding identifiers used for backend deduplication.
func reportsForStaff(reports []model.SpamReport) []model.SpamReport {
	visible := make([]model.SpamReport, len(reports))
	copy(visible, reports)
	for i := range visible {
		visible[i].ReporterUserID = ""
		visible[i].IdempotencyKey = ""
	}
	return visible
}

func AdminNumberDirectory(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireStaff(w, r, "MOD"); !ok {
		return
	}
	if r.Method != http.MethodGet {
		helper.WriteError(w, 405, "Gunakan GET")
		return
	}
	params := r.URL.Query()
	query := strings.TrimSpace(params.Get("q"))
	reputation := params.Get("reputation")
	offset := 0
	var err error
	if raw := params.Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || offset < 0 || len(query) > 160 {
		helper.WriteError(w, 400, "Pencarian atau halaman tidak valid")
		return
	}
	result, err := config.ListAdminNumbers(r.Context(), query, reputation, offset)
	if err != nil {
		adminCaseError(w, err)
		return
	}
	helper.WriteSuccess(w, 200, "Direktori reputasi nomor", result)
}
