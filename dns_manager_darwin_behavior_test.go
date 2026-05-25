//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type fakeDNSCommandRunner struct {
	outputs       map[string][]byte
	failRunAt     int
	runCount      int
	commands      []string
	canceledRuns  []string
	canceledCalls []string
}

func (r *fakeDNSCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	command := commandString(name, args...)
	r.commands = append(r.commands, command)
	r.runCount++
	if err := ctx.Err(); err != nil {
		r.canceledRuns = append(r.canceledRuns, command)
		return err
	}
	if r.failRunAt != 0 && r.runCount == r.failRunAt {
		return errors.New("set failed")
	}
	return nil
}

func (r *fakeDNSCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := commandString(name, args...)
	r.commands = append(r.commands, command)
	if err := ctx.Err(); err != nil {
		r.canceledCalls = append(r.canceledCalls, command)
		return nil, err
	}
	out, ok := r.outputs[command]
	if !ok {
		return nil, fmt.Errorf("unexpected output command: %s", command)
	}
	return out, nil
}

func TestDarwinDNSManagerApplyRecordsCommandsAndCleanupRestores(t *testing.T) {
	runner := newFakeDarwinDNSRunner()
	cleanup := NewCleanupStack()

	if err := (DarwinDNSManager{Runner: runner}).Apply(context.Background(), []string{"1.1.1.1", "8.8.8.8"}, cleanup); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup.Run() error = %v", err)
	}

	want := []string{
		"networksetup -listallnetworkservices",
		"networksetup -getdnsservers Wi-Fi",
		"networksetup -getdnsservers USB LAN",
		"networksetup -setdnsservers Wi-Fi 1.1.1.1 8.8.8.8",
		"networksetup -setdnsservers USB LAN 1.1.1.1 8.8.8.8",
		"networksetup -setdnsservers Wi-Fi Empty",
		"networksetup -setdnsservers USB LAN 9.9.9.9 149.112.112.112",
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %v, want %v", runner.commands, want)
	}
}

func TestDarwinDNSManagerSetFailureLeavesCleanupRegisteredWithFreshContext(t *testing.T) {
	runner := newFakeDarwinDNSRunner()
	runner.failRunAt = 2
	cleanup := NewCleanupStack()

	ctx, cancel := context.WithCancel(context.Background())
	err := (DarwinDNSManager{Runner: runner}).Apply(ctx, []string{"1.1.1.1"}, cleanup)
	if err == nil {
		t.Fatal("Apply() error = nil, want set failure")
	}
	if !strings.Contains(err.Error(), "set DNS servers for service USB LAN") {
		t.Fatalf("Apply() error = %v, want USB LAN set context", err)
	}

	cancel()
	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup.Run() error = %v", err)
	}
	if len(runner.canceledRuns) != 0 {
		t.Fatalf("cleanup used canceled context for %v", runner.canceledRuns)
	}

	wantTail := []string{
		"networksetup -setdnsservers Wi-Fi Empty",
		"networksetup -setdnsservers USB LAN 9.9.9.9 149.112.112.112",
	}
	gotTail := runner.commands[len(runner.commands)-len(wantTail):]
	if !reflect.DeepEqual(gotTail, wantTail) {
		t.Fatalf("cleanup commands = %v, want %v", gotTail, wantTail)
	}
}

func TestDarwinDNSManagerCleanupRunsAfterApplyContextCanceled(t *testing.T) {
	runner := newFakeDarwinDNSRunner()
	cleanup := NewCleanupStack()
	ctx, cancel := context.WithCancel(context.Background())

	if err := (DarwinDNSManager{Runner: runner}).Apply(ctx, []string{"1.1.1.1"}, cleanup); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	cancel()

	if err := cleanup.Run(); err != nil {
		t.Fatalf("cleanup.Run() error = %v", err)
	}
	if len(runner.canceledRuns) != 0 {
		t.Fatalf("cleanup used canceled context for %v", runner.canceledRuns)
	}
}

func TestDarwinDNSManagerCleanupAttemptsAllRestoresAfterFailure(t *testing.T) {
	runner := newFakeDarwinDNSRunner()
	runner.failRunAt = 4
	cleanup := NewCleanupStack()

	if err := (DarwinDNSManager{Runner: runner}).Apply(context.Background(), []string{"1.1.1.1"}, cleanup); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	err := cleanup.Run()
	if err == nil {
		t.Fatalf("cleanup.Run() error = nil, want restore failure")
	}
	if !strings.Contains(err.Error(), `manual recovery: networksetup "-setdnsservers" "USB LAN" "9.9.9.9" "149.112.112.112"`) {
		t.Fatalf("cleanup.Run() error = %v, want manual recovery command", err)
	}

	wantTail := []string{
		"networksetup -setdnsservers Wi-Fi Empty",
		"networksetup -setdnsservers USB LAN 9.9.9.9 149.112.112.112",
	}
	gotTail := runner.commands[len(runner.commands)-len(wantTail):]
	if !reflect.DeepEqual(gotTail, wantTail) {
		t.Fatalf("cleanup commands = %v, want %v", gotTail, wantTail)
	}
}

func newFakeDarwinDNSRunner() *fakeDNSCommandRunner {
	return &fakeDNSCommandRunner{
		outputs: map[string][]byte{
			"networksetup -listallnetworkservices": []byte("An asterisk (*) denotes that a network service is disabled.\nWi-Fi\nUSB LAN\n"),
			"networksetup -getdnsservers Wi-Fi":    []byte("There aren't any DNS Servers set on Wi-Fi.\n"),
			"networksetup -getdnsservers USB LAN":  []byte("9.9.9.9\n149.112.112.112\n"),
		},
	}
}
