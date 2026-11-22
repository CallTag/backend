package helper

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCarrierContextSharedAndroidFixture(t *testing.T) {
	raw, err := os.ReadFile("../testdata/carrier_context.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Phone    string `json:"phone"`
		Expected string `json:"expected"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 400 {
		t.Fatal("fixture tabel prefix tidak lengkap")
	}
	for _, tc := range cases {
		if got := DetectIndonesianCarrier(tc.Phone); got != tc.Expected {
			t.Errorf("%s: got %q; want %q", tc.Phone, got, tc.Expected)
		}
	}
}
