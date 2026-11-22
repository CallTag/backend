package helper

import "testing"

func TestPIISanitizerPreservesServiceContext(t *testing.T) {
	for _, label := range []string{"Kurir Ekspedisi", "Layanan Pelanggan", "Admin Toko", "Sales Properti"} {
		if got := SanitizeCommentPII(label); got != label {
			t.Fatalf("label berubah: %q", got)
		}
	}
}
