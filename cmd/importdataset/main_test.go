package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDryRunWorksWithoutDatabase(t *testing.T) {
	t.Setenv("MONGOSTRING", "")
	t.Setenv("MONGODB_URI", "")
	file := filepath.Join(t.TempDir(), "sample.csv")
	if err := os.WriteFile(file, []byte("phone_e164,category_code,custom_tag\n081234567899,INFO_TAG,Kurir\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"-file", file, "-kind", "tag_context", "-source", "TEAM", "-dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"validated":1`)) {
		t.Fatal(out.String())
	}
	if err := run([]string{"-file", file, "-kind", "tag_context", "-source", "TEAM"}, &out); err == nil {
		t.Fatal("write without MongoDB must fail")
	}
}
