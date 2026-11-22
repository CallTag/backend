package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSeedRejectsCredentialsInArguments(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"-password", "secret"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("accepted secret argv")
	}
	if strings.Contains(out.String(), "secret") {
		t.Fatal("leaked secret")
	}
}
