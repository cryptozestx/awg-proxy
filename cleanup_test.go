package main

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanupStackRunsInReverseOrder(t *testing.T) {
	stack := NewCleanupStack()
	var calls []string

	stack.Add("first", func() error {
		calls = append(calls, "first")
		return nil
	})
	stack.Add("second", func() error {
		calls = append(calls, "second")
		return nil
	})

	if err := stack.Run(); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"second", "first"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", calls, want)
		}
	}
}

func TestCleanupStackIsIdempotent(t *testing.T) {
	stack := NewCleanupStack()
	calls := 0

	stack.Add("only", func() error {
		calls++
		return nil
	})

	if err := stack.Run(); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := stack.Run(); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestCleanupStackAggregatesErrors(t *testing.T) {
	stack := NewCleanupStack()

	stack.Add("route", func() error {
		return errors.New("route delete failed")
	})
	stack.Add("dns", func() error {
		return errors.New("dns restore failed")
	})

	err := stack.Run()
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}

	errText := err.Error()
	for _, want := range []string{"route delete failed", "dns restore failed"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("Run() error = %q, want it to contain %q", errText, want)
		}
	}
}
