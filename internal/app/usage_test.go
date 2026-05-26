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
