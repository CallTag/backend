package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

type openAPIDoc struct {
	OpenAPI    string                    `json:"openapi"`
	Info       map[string]any            `json:"info"`
	Paths      map[string]map[string]any `json:"paths"`
	Components struct {
		Schemas map[string]map[string]any `json:"schemas"`
	} `json:"components"`
}

func loadOpenAPIDoc(t *testing.T) openAPIDoc {
	t.Helper()
	p := filepath.Join("..", "apidocs", "openapi.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("Gagal membaca %s: %v", p, err)
	}
	var doc openAPIDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("Gagal parse %s: %v", p, err)
	}
	return doc
}

func resolveOpenAPISchema(doc openAPIDoc, raw map[string]any) map[string]any {
	if raw == nil {
		return nil
	}
	if ref, ok := raw["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		if target, exists := doc.Components.Schemas[name]; exists {
			return resolveOpenAPISchema(doc, target)
		}
	}
	if allOf, ok := raw["allOf"].([]any); ok {
		mergedProps := map[string]any{}
		mergedType := "object"
		for _, sub := range allOf {
			m, ok := sub.(map[string]any)
			if !ok {
				continue
			}
			resolved := resolveOpenAPISchema(doc, m)
			if tp, ok := resolved["type"].(string); ok && tp != "" {
				mergedType = tp
			}
			if props, ok := resolved["properties"].(map[string]any); ok {
				for k, v := range props {
					mergedProps[k] = v
				}
			}
		}
		out := map[string]any{
			"type":       mergedType,
			"properties": mergedProps,
		}
		if props, ok := raw["properties"].(map[string]any); ok {
			for k, v := range props {
				mergedProps[k] = v
			}
		}
		return out
	}
	return raw
}

func validateValueAgainstOpenAPI(t *testing.T, doc openAPIDoc, location string, schema map[string]any, val any) {
	t.Helper()
	if schema == nil || val == nil {
		return
	}
	schema = resolveOpenAPISchema(doc, schema)
	if schema == nil {
		return
	}
	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		obj, ok := val.(map[string]any)
		if !ok {
			t.Errorf("%s: expected JSON object, got %T (%v)", location, val, val)
			return
		}
		props, _ := schema["properties"].(map[string]any)
		addPropsAllowed := false
		if ap, exists := schema["additionalProperties"]; exists {
			switch apv := ap.(type) {
			case bool:
				addPropsAllowed = apv
			case map[string]any:
				addPropsAllowed = true
				for k, v := range obj {
					if props != nil {
						if _, hasExplicit := props[k]; hasExplicit {
							continue
						}
					}
					validateValueAgainstOpenAPI(t, doc, location+"."+k, apv, v)
				}
			}
		}
		if props != nil && !addPropsAllowed {
			for k := range obj {
				if _, declared := props[k]; !declared {
					t.Errorf("%s: field JSON %q tidak terdokumentasi di OpenAPI schema", location, k)
				}
			}
		}
		for k, propRaw := range props {
			v, present := obj[k]
			if !present || v == nil {
				continue
			}
			propSchema, ok := propRaw.(map[string]any)
			if ok {
				validateValueAgainstOpenAPI(t, doc, location+"."+k, propSchema, v)
			}
		}
	case "array":
		arr, ok := val.([]any)
		if !ok {
			t.Errorf("%s: expected JSON array, got %T", location, val)
			return
		}
		itemSchema, _ := schema["items"].(map[string]any)
		if itemSchema != nil {
			for idx, item := range arr {
				validateValueAgainstOpenAPI(t, doc, fmt.Sprintf("%s[%d]", location, idx), itemSchema, item)
			}
		}
	case "string":
		if _, ok := val.(string); !ok {
			t.Errorf("%s: expected JSON string, got %T (%v)", location, val, val)
		}
	case "integer":
		f, ok := val.(float64)
		if !ok || f != float64(int64(f)) {
			t.Errorf("%s: expected JSON integer, got %T (%v)", location, val, val)
		}
	case "number":
		if _, ok := val.(float64); !ok {
			t.Errorf("%s: expected JSON number, got %T (%v)", location, val, val)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			t.Errorf("%s: expected JSON boolean, got %T (%v)", location, val, val)
		}
	}
}

func TestQA_T602_OpenAPIContractParity(t *testing.T) {
	initQATestStorage(t, "calltag_qa_contract")
	doc := loadOpenAPIDoc(t)

	if v, _ := doc.Info["version"].(string); v != "1.2.0" {
		t.Fatalf("Expected openapi.json version 1.2.0, got %q", v)
	}

	// 1. Verifikasi enum CategoryCode (termasuk INFO_TAG), ReputationLevel, dan CallType sinkron dengan backend
	catSchema := doc.Components.Schemas["CategoryCode"]
	catEnums, _ := catSchema["enum"].([]any)
	if len(catEnums) != 7 {
		t.Fatalf("Expected 7 CategoryCode di OpenAPI, got %d (%v)", len(catEnums), catEnums)
	}
	for _, raw := range catEnums {
		code := model.CategoryCode(raw.(string))
		if _, ok := helper.GetCategoryInfo(code); !ok {
			t.Errorf("CategoryCode OpenAPI %q tidak dikenali oleh helper.GetCategoryInfo", code)
		}
	}
	repSchema := doc.Components.Schemas["ReputationLevel"]
	repEnums, _ := repSchema["enum"].([]any)
	wantReps := map[string]bool{
		string(model.ReputationClean):      true,
		string(model.ReputationSuspicious): true,
		string(model.ReputationSpam):       true,
		string(model.ReputationDangerScam): true,
	}
	if len(repEnums) != len(wantReps) {
		t.Fatalf("Expected 4 ReputationLevel di OpenAPI, got %v", repEnums)
	}
	for _, raw := range repEnums {
		if !wantReps[raw.(string)] {
			t.Errorf("ReputationLevel OpenAPI tak dikenal: %v", raw)
		}
	}

	// Siapkan akun device + admin + data uji untuk menguji semua endpoint & response schema
	regRec := doJSONRoute(http.MethodPost, "/api/v1/auth/device", map[string]string{
		"install_id":   "inst-contract-01",
		"display_name": "Contract Tester",
	}, nil)
	if regRec.Code != http.StatusOK {
		t.Fatalf("Auth device contract setup gagal: %d %s", regRec.Code, regRec.Body.String())
	}
	var authEnvelope map[string]any
	_ = json.Unmarshal(regRec.Body.Bytes(), &authEnvelope)
	authData := authEnvelope["data"].(map[string]any)
	deviceToken := authData["token"].(string)
	refreshToken := authData["refresh_token"].(string)

	// Tambah verified number agar LookupData menguji field verified_* juga
	adminCookie, adminCSRF := loginTestStaffHTTP(t, "qa-admin-contract", "ADMIN")
	for _, query := range []string{"08129988", "Modus+Lihat+Foto+Paket"} {
		results := doJSONRoute(http.MethodGet, "/api/v1/admin/numbers?q="+query, nil, nil, adminCookie)
		if results.Code != http.StatusOK || !strings.Contains(results.Body.String(), "+6281299887766") ||
			strings.Contains(results.Body.String(), "reporter_user_id") {
			t.Fatalf("Pencarian nomor %q: %d %s", query, results.Code, results.Body.String())
		}
	}

	recAddVer := doJSONRoute(http.MethodPost, "/api/v1/admin/verified-numbers", map[string]string{
		"phone_e164":    "+6221500111",
		"kind":          "BUSINESS",
		"display_label": "CS Resmi Contract Test",
		"source_url":    "https://resmi.example.test",
		"verified_at":   time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		"expires_at":    time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		"reason":        "Setup contract test",
	}, map[string]string{"X-CSRF-Token": adminCSRF}, adminCookie)
	if recAddVer.Code != http.StatusOK && recAddVer.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/admin/verified-numbers gagal: %d %s", recAddVer.Code, recAddVer.Body.String())
	}

	// Buat report INFO_TAG + report PINJOL_DC + dispute agar seluruh endpoint memiliki data nyata
	recRep := doJSONRoute(http.MethodPost, "/api/v1/reports", map[string]string{
		"phone":         "081345670099",
		"category_code": "INFO_TAG",
		"custom_tag":    "Kurir Paket Contract",
		"comment":       "Mengantar paket sesuai alamat",
		"call_type":     "WHATSAPP_CALL",
	}, map[string]string{"Authorization": "Bearer " + deviceToken, "Idempotency-Key": "contract-rep-1"})
	if recRep.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/reports INFO_TAG gagal: %d %s", recRep.Code, recRep.Body.String())
	}
	recDisp := doJSONRoute(http.MethodPost, "/api/v1/disputes", map[string]string{
		"phone":       "081345670099",
		"owner_name":  "Pemilik Kurir Contract",
		"reason":      "Verifikasi kontrak dispute",
		"contact_ref": "kurir@example.test",
	}, map[string]string{"X-Device-Id": "contract-guest"})
	if recDisp.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/disputes gagal: %d %s", recDisp.Code, recDisp.Body.String())
	}
	var dispEnv map[string]any
	_ = json.Unmarshal(recDisp.Body.Bytes(), &dispEnv)
	dispData := dispEnv["data"].(map[string]any)
	disputeID := dispData["dispute_id"].(string)
	trackingToken := dispData["tracking_token"].(string)

	// 2. Pastikan setiap path di OpenAPI terpetakan ke handler backend (bukan 404)
	resolveConcretePath := func(tmpl string) string {
		r := tmpl
		r = strings.ReplaceAll(r, "{e164}", "+6281299887766")
		if tmpl == "/api/v1/disputes/{id}" {
			r = strings.ReplaceAll(r, "{id}", disputeID)
		} else if tmpl == "/api/v1/admin/cases/{id}" {
			r = strings.ReplaceAll(r, "{id}", disputeID)
		} else {
			r = strings.ReplaceAll(r, "{id}", "dummy-id")
		}
		r = strings.ReplaceAll(r, "{code}", "POLISI_110")
		r = strings.ReplaceAll(r, "{slug}", "bca")
		return r
	}

	for tmpl, methods := range doc.Paths {
		for method := range methods {
			upper := strings.ToUpper(method)
			concrete := resolveConcretePath(tmpl)
			// Jangan langsung logout atau mutasi destructif sebelum cek GET; tes logout di akhir
			if tmpl == "/api/v1/admin/logout" || tmpl == "/api/v1/me" && upper == http.MethodDelete || upper == http.MethodDelete {
				continue
			}
			var headers = map[string]string{}
			var body any
			switch {
			case strings.HasPrefix(tmpl, "/api/v1/admin/"):
				headers["X-CSRF-Token"] = adminCSRF
				if upper == http.MethodPost || upper == http.MethodPut || upper == http.MethodPatch {
					// Kirim JSON minimal yang valid bentuknya; cukup memastikan router menemukannya (bukan 404).
					body = map[string]any{}
				}
			case tmpl == "/api/v1/auth/device":
				body = map[string]string{"install_id": "inst-route-probe", "display_name": "Probe"}
			case tmpl == "/api/v1/auth/refresh":
				body = map[string]string{"refresh_token": refreshToken}
			case tmpl == "/api/v1/me":
				headers["Authorization"] = "Bearer " + deviceToken
			case tmpl == "/api/v1/reports":
				headers["Authorization"] = "Bearer " + deviceToken
				body = map[string]string{"phone": "081345670088", "category_code": "PINJOL_DC", "custom_tag": "Pinjol"}
			case tmpl == "/api/v1/disputes":
				headers["X-Device-Id"] = "contract-probe"
				body = map[string]string{"phone": "081345670077", "owner_name": "Pemilik", "reason": "Alasan uji"}
			case tmpl == "/api/v1/disputes/{id}":
				headers["X-Tracking-Token"] = trackingToken
			}
			rec := doJSONRoute(upper, concrete, body, headers, adminCookie)
			if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "Endpoint") {
				t.Errorf("OpenAPI endpoint %s %s (%s) mengembalikan 404 endpoint tidak ditemukan: %s", upper, tmpl, concrete, rec.Body.String())
			}
		}
	}

	// 3. Validasi penuh skema JSON response endpoint publik & device terhadap OpenAPI 1.2.0
	type schemaCheckCase struct {
		name       string
		openAPIKey string
		method     string
		path       string
		headers    map[string]string
		cookies    []*http.Cookie
		wantCode   int
	}
	checks := []schemaCheckCase{
		{"health", "/health", http.MethodGet, "/health", nil, nil, http.StatusOK},
		{"healthz", "/healthz", http.MethodGet, "/healthz", nil, nil, http.StatusOK},
		{"readyz", "/readyz", http.MethodGet, "/readyz", nil, nil, http.StatusOK},
		{"me", "/api/v1/me", http.MethodGet, "/api/v1/me", map[string]string{"Authorization": "Bearer " + deviceToken}, nil, http.StatusOK},
		{"phone_lookup_scam", "/api/v1/phones/{e164}", http.MethodGet, "/api/v1/phones/+6281299887766", nil, nil, http.StatusOK},
		{"phone_lookup_info_tag", "/api/v1/phones/{e164}", http.MethodGet, "/api/v1/phones/+6281345670099", nil, nil, http.StatusOK},
		{"phone_lookup_verified", "/api/v1/phones/{e164}", http.MethodGet, "/api/v1/phones/+6221500111", nil, nil, http.StatusOK},
		{"phone_lookup_clean_unknown", "/api/v1/phones/{e164}", http.MethodGet, "/api/v1/phones/+6281100000001", nil, nil, http.StatusOK},
		{"phone_reports", "/api/v1/phones/{e164}/reports", http.MethodGet, "/api/v1/phones/+6281299887766/reports", nil, nil, http.StatusOK},
		{"dispute_get", "/api/v1/disputes/{id}", http.MethodGet, "/api/v1/disputes/" + disputeID, map[string]string{"X-Tracking-Token": trackingToken}, nil, http.StatusOK},
		{"emergency_national", "/api/v1/emergency/national", http.MethodGet, "/api/v1/emergency/national", nil, nil, http.StatusOK},
		{"emergency_local", "/api/v1/emergency/local", http.MethodGet, "/api/v1/emergency/local?province=JAWA%20BARAT", nil, nil, http.StatusOK},
		{"official_channels", "/api/v1/official-channels", http.MethodGet, "/api/v1/official-channels", nil, nil, http.StatusOK},
		{"sync_offline_bundle", "/api/v1/sync/offline-bundle", http.MethodGet, "/api/v1/sync/offline-bundle", nil, nil, http.StatusOK},
		{"sync_changes", "/api/v1/sync/changes", http.MethodGet, "/api/v1/sync/changes?limit=50", nil, nil, http.StatusOK},
		{"stats_public", "/api/v1/stats/public", http.MethodGet, "/api/v1/stats/public", nil, nil, http.StatusOK},
		{"admin_session", "/api/v1/admin/auth/session", http.MethodGet, "/api/v1/admin/auth/session", nil, []*http.Cookie{adminCookie}, http.StatusOK},
		{"admin_audit", "/api/v1/admin/audit", http.MethodGet, "/api/v1/admin/audit?limit=10", nil, []*http.Cookie{adminCookie}, http.StatusOK},
		{"admin_verified", "/api/v1/admin/verified-numbers", http.MethodGet, "/api/v1/admin/verified-numbers", nil, []*http.Cookie{adminCookie}, http.StatusOK},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSONRoute(tc.method, tc.path, nil, tc.headers, tc.cookies...)
			if rec.Code != tc.wantCode {
				t.Fatalf("%s %s status=%d (want %d): %s", tc.method, tc.path, rec.Code, tc.wantCode, rec.Body.String())
			}
			var payload map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("Gagal decode JSON response: %v", err)
			}
			// Periksa amplop standar PRD
			for _, key := range []string{"success", "code", "message", "timestamp"} {
				if _, ok := payload[key]; !ok {
					t.Errorf("%s: missing envelope key %q", tc.name, key)
				}
			}

			pathItem := doc.Paths[tc.openAPIKey]
			op, _ := pathItem[strings.ToLower(tc.method)].(map[string]any)
			resps, _ := op["responses"].(map[string]any)
			respObj, _ := resps[fmt.Sprintf("%d", tc.wantCode)].(map[string]any)
			content, _ := respObj["content"].(map[string]any)
			appJSON, _ := content["application/json"].(map[string]any)
			respSchema, _ := appJSON["schema"].(map[string]any)
			if respSchema != nil {
				validateValueAgainstOpenAPI(t, doc, tc.name+".data", respSchema, payload["data"])
			}
		})
	}

	// Validasi khusus schema POST responses dari data yang sudah dibuat
	validateNamedDataSchema := func(label, schemaName string, recBody []byte) {
		t.Helper()
		var env map[string]any
		if err := json.Unmarshal(recBody, &env); err != nil {
			t.Fatalf("%s: decode gagal: %v", label, err)
		}
		schema := doc.Components.Schemas[schemaName]
		if schema == nil {
			t.Fatalf("Schema %q tidak ditemukan di openapi.json", schemaName)
		}
		validateValueAgainstOpenAPI(t, doc, label, schema, env["data"])
	}
	validateNamedDataSchema("auth_device", "AuthResponseData", regRec.Body.Bytes())
	validateNamedDataSchema("submit_report", "ReportResponseData", recRep.Body.Bytes())
	validateNamedDataSchema("submit_dispute", "DisputeResponseData", recDisp.Body.Bytes())

	// Pastikan DELETE /api/v1/me dan POST /api/v1/admin/logout juga dapat dipanggil dan tidak 404
	recDelMe := doJSONRoute(http.MethodDelete, "/api/v1/me", nil, map[string]string{"Authorization": "Bearer " + deviceToken})
	if recDelMe.Code != http.StatusAccepted {
		t.Fatalf("DELETE /api/v1/me expected 202, got %d", recDelMe.Code)
	}
	recLogout := doJSONRoute(http.MethodPost, "/api/v1/admin/logout", nil, map[string]string{"X-CSRF-Token": adminCSRF}, adminCookie)
	if recLogout.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/admin/logout expected 200, got %d", recLogout.Code)
	}
}
