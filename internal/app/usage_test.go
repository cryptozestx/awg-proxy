package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintUsageListsTunnelOptions(t *testing.T) {
	var out bytes.Buffer
	PrintUsage(&out)

	text := out.String()
	for _, flag := range []string{"--rules", "--dry-run", "--no-dns", "--verbose"} {
		if !strings.Contains(text, flag) {
			t.Fatalf("usage output does not mention %s:\n%s", flag, text)
		}
	}
}

func TestPrintUsageDoesNotDoublePrefixInjectedVersion(t *testing.T) {
	original := Version
	Version = "v9.9.9"
	t.Cleanup(func() {
		Version = original
	})

	var out bytes.Buffer
	PrintUsage(&out)

	text := out.String()
	if !strings.Contains(text, "v9.9.9") {
		t.Fatalf("usage output does not mention injected version:\n%s", text)
	}
	if strings.Contains(text, "vv9.9.9") {
		t.Fatalf("usage output double-prefixed injected version:\n%s", text)
	}
}
