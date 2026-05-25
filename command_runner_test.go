package main

import (
	"reflect"
	"testing"
)

func TestDryRunRunnerRecordsCommands(t *testing.T) {
	r := NewDryRunRunner()

	if err := r.Run("route", "add", "0.0.0.0/1", "-interface", "utun7"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"route add 0.0.0.0/1 -interface utun7"}
	if !reflect.DeepEqual(r.Commands(), want) {
		t.Fatalf("Commands() = %v, want %v", r.Commands(), want)
	}
}
