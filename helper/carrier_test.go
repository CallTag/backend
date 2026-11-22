package helper

import (
	"strings"
	"testing"
)

func TestDetectCarrierRegionalAndInternationalContext(t *testing.T) {
	cases := []struct{ phone, want string }{
		{"+622513456789", "Bogor"}, {"+622803456789", "Majenang"},
		{"+623703456789", "Mataram"}, {"+624013456789", "Kendari"},
		{"+625613456789", "Pontianak"}, {"+626513456789", "Banda Aceh"},
		{"+627783456789", "Batam"}, {"+629013456789", "Timika"},
		{"+629673456789", "Jayapura"}, {"+629843456789", "Nabire"},
		{"+629863456789", "Manokwari"}, {"+628111234567", "Telkomsel"},
		{"+628511234567", "by.U"}, {"+628141234567", "Indosat"},
		{"+628951234567", "Tri"}, {"+628311234567", "AXIS"},
		{"+628881234567", "Smartfren"}, {"+628771234567", "XL"},
	}
	for _, tt := range cases {
		t.Run(tt.phone, func(t *testing.T) {
			if got := DetectIndonesianCarrier(tt.phone); !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	countries := []struct{ prefix, name string }{{"+65", "Singapura"}, {"+60", "Malaysia"}, {"+63", "Filipina"}, {"+855", "Kamboja"}, {"+95", "Myanmar"}, {"+1", "Amerika Utara"}, {"+81", "Jepang"}, {"+7", "Rusia/Kazakhstan"}, {"+675", "Papua Nugini"}}
	for _, tt := range countries {
		t.Run(tt.prefix, func(t *testing.T) {
			got := DetectIndonesianCarrier(tt.prefix + "123456789")
			if !strings.Contains(got, tt.name) || !strings.Contains(got, "bukan bukti spam") {
				t.Errorf("missing neutral international context: %q", got)
			}
		})
	}
}

func TestDetectCarrierDoesNotInventEmergencyOrDomesticOrigin(t *testing.T) {
	for _, phone := range []string{"", "1", "999999", "+999123456789", "+65", "+62"} {
		got := DetectIndonesianCarrier(phone)
		if strings.Contains(got, "Darurat") || strings.Contains(got, "Telekomunikasi Indonesia") {
			t.Errorf("%q incorrectly classified: %q", phone, got)
		}
	}
	if got := DetectIndonesianCarrier("112"); !strings.Contains(got, "Darurat") {
		t.Errorf("112: %q", got)
	}
	if got := DetectIndonesianCarrier("+628001234567"); got != "Operator Telekomunikasi Indonesia" {
		t.Errorf("unknown domestic: %q", got)
	}
}

func TestNormalizePhoneIDPreservesExplicitInternationalCode(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{"+855 12-345-678", "+85512345678"}, {"+81 90 1234 5678", "+819012345678"},
		{"+82 10 1234 5678", "+821012345678"}, {"0812 3456 7890", "+6281234567890"},
		{"81234567890", "+6281234567890"}, {"+65", "+65"}, {"112", "112"},
	} {
		if got := NormalizePhoneID(tt.raw); got != tt.want {
			t.Errorf("NormalizePhoneID(%q)=%q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestNormalizeNationalServiceNumber(t *testing.T) {
	if got := NormalizePhoneID("1500888"); got != "1500888" {
		t.Errorf("got %q", got)
	}
}
