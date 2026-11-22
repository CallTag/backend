// Package dataset validates public dataset imports before storage is touched.
package dataset

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
)

type Record struct {
	PhoneE164    string             `json:"phone_e164"`
	Kind         string             `json:"kind,omitempty"`
	DisplayLabel string             `json:"display_label,omitempty"`
	SourceURL    string             `json:"source_url,omitempty"`
	VerifiedAt   time.Time          `json:"verified_at,omitempty"`
	ExpiresAt    time.Time          `json:"expires_at,omitempty"`
	CategoryCode model.CategoryCode `json:"category_code,omitempty"`
	CustomTag    string             `json:"custom_tag,omitempty"`
	Comment      string             `json:"comment,omitempty"`
	Source       string             `json:"source"`
}

var verifiedCanonical = regexp.MustCompile(`^(\+[1-9][0-9]{6,14}|[1-9][0-9]{2,5}|150[0-9]{4})$`)

var phoneInput = regexp.MustCompile(`^\+?[0-9][0-9 ()\-.]*$`)
var sourceLabel = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_:.\-]{0,79}$`)

// Parse accepts template CSV/JSON arrays and CallShield's numbers envelope.
// All rows are validated first; a bad row prevents the entire import.
func Parse(r io.Reader, extension, kind, source string) ([]Record, error) {
	if kind != "verified" && kind != "scam" && kind != "tag_context" {
		return nil, errors.New("kind must be verified, scam or tag_context")
	}
	data, err := io.ReadAll(io.LimitReader(r, 32*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 32*1024*1024 {
		return nil, errors.New("dataset exceeds 32 MiB; use curated subsets")
	}
	r = strings.NewReader(string(data))
	var rows []Record
	switch strings.ToLower(extension) {
	case ".csv":
		reader := csv.NewReader(r)
		header, err := reader.Read()
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for i := range header {
			header[i] = strings.TrimSpace(strings.TrimPrefix(header[i], "\ufeff"))
			if seen[header[i]] {
				return nil, errors.New("duplicate CSV header")
			}
			seen[header[i]] = true
		}
		for n := 2; ; n++ {
			cells, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("row %d: invalid CSV", n)
			}
			fields := map[string]string{}
			for i, k := range header {
				fields[k] = cells[i]
			}
			verifiedAt, _ := time.Parse(time.RFC3339, fields["verified_at"])
			expiresAt, _ := time.Parse(time.RFC3339, fields["expires_at"])
			rows = append(rows, Record{VerifiedAt: verifiedAt, ExpiresAt: expiresAt, PhoneE164: fields["phone_e164"], Kind: fields["kind"], DisplayLabel: fields["display_label"], SourceURL: fields["source_url"], CategoryCode: model.CategoryCode(fields["category_code"]), CustomTag: fields["custom_tag"], Comment: fields["comment"], Source: fields["source"]})
		}
	case ".json":
		data = []byte(strings.TrimSpace(string(data)))
		if len(data) == 0 {
			return nil, errors.New("empty JSON")
		}
		if data[0] == '[' {
			if err := json.Unmarshal(data, &rows); err != nil {
				return nil, errors.New("invalid record array")
			}
		} else {
			if kind != "scam" || source != "EXTERNAL_OPEN_LIST" {
				return nil, errors.New("CallShield format requires scam and source EXTERNAL_OPEN_LIST")
			}
			var envelope struct {
				Numbers []struct {
					Number      string `json:"number"`
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"numbers"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil {
				return nil, errors.New("invalid CallShield JSON")
			}
			for _, n := range envelope.Numbers {
				category := model.CategoryTelemarketing
				if n.Type == "scam" || n.Type == "fraud" {
					category = model.CategorySocialEng
				}
				rows = append(rows, Record{PhoneE164: n.Number, CategoryCode: category, CustomTag: "External open list: " + n.Type, Comment: n.Description, Source: source})
			}
		}
	default:
		return nil, errors.New("file must have .csv or .json extension")
	}
	if len(rows) == 0 {
		return nil, errors.New("dataset contains no records")
	}
	out := make([]Record, 0, len(rows))
	seen := map[string]bool{}
	for i, row := range rows {
		if source != "" {
			row.Source = source
		}
		row.Source = strings.ToUpper(strings.TrimSpace(row.Source))
		if !sourceLabel.MatchString(row.Source) {
			return nil, fmt.Errorf("row %d: source label is required (A-Z, 0-9, _ . : -)", i+1)
		}
		raw := strings.TrimSpace(row.PhoneE164)
		if !phoneInput.MatchString(raw) {
			return nil, fmt.Errorf("row %d: invalid phone syntax", i+1)
		}
		row.PhoneE164 = helper.NormalizePhoneID(raw)
		digits := strings.TrimPrefix(row.PhoneE164, "+")
		if len(digits) < 3 || len(digits) > 15 || strings.HasPrefix(digits, "0") || (kind != "verified" && len(digits) < 8) {
			return nil, fmt.Errorf("row %d: invalid phone length", i+1)
		}
		if kind == "verified" {
			if !verifiedCanonical.MatchString(row.PhoneE164) {
				return nil, fmt.Errorf("row %d: invalid verified phone", i+1)
			}
			if row.Source == "EXTERNAL_OPEN_LIST" {
				return nil, fmt.Errorf("row %d: external lists cannot grant verified status", i+1)
			}
			row.Kind = strings.ToUpper(strings.TrimSpace(row.Kind))
			if row.Kind != "OFFICIAL" && row.Kind != "BUSINESS" && row.Kind != "PUBLIC_SERVICE" {
				return nil, fmt.Errorf("row %d: invalid verified kind", i+1)
			}
			u, err := url.Parse(strings.TrimSpace(row.SourceURL))
			if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
				return nil, fmt.Errorf("row %d: source_url must be a public official web URL", i+1)
			}
			if row.VerifiedAt.IsZero() || !row.ExpiresAt.After(row.VerifiedAt) {
				return nil, fmt.Errorf("row %d: verified_at and expires_at RFC3339 dates are required", i+1)
			}
			row.SourceURL = u.String()
			row.DisplayLabel = helper.SanitizeCommentPII(row.DisplayLabel)
			if row.DisplayLabel == "" || utf8.RuneCountInString(row.DisplayLabel) > 120 {
				return nil, fmt.Errorf("row %d: invalid display_label", i+1)
			}
		} else {
			row.CategoryCode = model.CategoryCode(strings.ToUpper(strings.TrimSpace(string(row.CategoryCode))))
			if _, ok := helper.GetCategoryInfo(row.CategoryCode); !ok {
				return nil, fmt.Errorf("row %d: unknown category_code", i+1)
			}
			if (kind == "tag_context") != (row.CategoryCode == model.CategoryCode("INFO_TAG")) {
				return nil, fmt.Errorf("row %d: category does not match import kind", i+1)
			}
			row.CustomTag = helper.SanitizeCommentPII(row.CustomTag)
			row.Comment = helper.SanitizeCommentPII(row.Comment)
			if row.CustomTag == "" || utf8.RuneCountInString(row.CustomTag) > 40 || utf8.RuneCountInString(row.Comment) > 1000 {
				return nil, fmt.Errorf("row %d: tag/comment exceeds allowed size or empty tag", i+1)
			}
		}
		b, _ := json.Marshal(row)
		key := string(b)
		if !seen[key] {
			out = append(out, row)
			seen[key] = true
		}
	}
	return out, nil
}
