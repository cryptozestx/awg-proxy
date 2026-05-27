package app

import (
	"bytes"
	"errors"
	"testing"
)

func TestRunReturnsUsageErrorForInvalidCLI(t *testing.T) {
	var stderr bytes.Buffer
	err := Runtime{Stderr: &stderr}.Run([]string{"awg-proxy"})
	if err == nil {
		t.Fatalf("Run succeeded, want error")
	}

	var usageErr UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run error = %T, want UsageError", err)
	}
	if !usageErr.BlankLineBeforeUsage {
		t.Fatalf("BlankLineBeforeUsage = false, want true")
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run wrote usage directly to stderr; entrypoint should order error before usage")
	}
}

func TestRunReturnsUsageErrorForMissingConfig(t *testing.T) {
	err := Runtime{}.Run([]string{"awg-proxy", "shell", "-c", ""})
	if err == nil {
		t.Fatalf("Run succeeded, want error")
	}

	var usageErr UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("Run error = %T, want UsageError", err)
	}
	if usageErr.BlankLineBeforeUsage {
		t.Fatalf("BlankLineBeforeUsage = true, want false")
	}
}
