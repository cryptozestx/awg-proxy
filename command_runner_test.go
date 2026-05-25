package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDryRunRunnerRecordsCommands(t *testing.T) {
	r := NewDryRunRunner()

	if err := r.Run(context.Background(), "route", "add", "0.0.0.0/1", "-interface", "utun7"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"route add 0.0.0.0/1 -interface utun7"}
	if !reflect.DeepEqual(r.Commands(), want) {
		t.Fatalf("Commands() = %v, want %v", r.Commands(), want)
	}
}

func TestDryRunRunnerOutputRecordsCommandAndReturnsUnavailable(t *testing.T) {
	r := NewDryRunRunner()

	output, err := r.Output(context.Background(), "route", "-n", "get", "203.0.113.10")
	if !errors.Is(err, ErrDryRunOutputUnavailable) {
		t.Fatalf("Output() error = %v, want %v", err, ErrDryRunOutputUnavailable)
	}
	if output != nil {
		t.Fatalf("Output() = %v, want nil", output)
	}

	want := []string{"route -n get 203.0.113.10"}
	if !reflect.DeepEqual(r.Commands(), want) {
		t.Fatalf("Commands() = %v, want %v", r.Commands(), want)
	}
}

func TestDryRunRunnerCommandsReturnsDefensiveCopy(t *testing.T) {
	r := NewDryRunRunner()

	if err := r.Run(context.Background(), "route", "add", "0.0.0.0/1"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	commands := r.Commands()
	commands[0] = "mutated"

	want := []string{"route add 0.0.0.0/1"}
	if !reflect.DeepEqual(r.Commands(), want) {
		t.Fatalf("Commands() = %v, want %v", r.Commands(), want)
	}
}
