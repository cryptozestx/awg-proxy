package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestPrintUsageListsTunnelOptions(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})

	printUsage()
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("io.ReadAll returned error: %v", err)
	}

	text := string(out)
	for _, flag := range []string{"--rules", "--dry-run", "--no-dns", "--verbose"} {
		if !strings.Contains(text, flag) {
			t.Fatalf("usage output does not mention %s:\n%s", flag, text)
		}
	}
}
