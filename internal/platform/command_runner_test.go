package platform

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeOutputRunner struct {
	output []byte
	err    error
	called bool
	name   string
	args   []string
}

func (r *fakeOutputRunner) Run(context.Context, string, ...string) error {
	return nil
}

func (r *fakeOutputRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.called = true
	r.name = name
	r.args = append([]string(nil), args...)
	return r.output, r.err
}

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

func TestDryRunRunnerOutputCanDelegateReadOnlyDiscovery(t *testing.T) {
	outputRunner := &fakeOutputRunner{output: []byte("gateway: 192.0.2.1\n")}
	r := NewDryRunRunnerWithOutput(outputRunner)

	output, err := r.Output(context.Background(), "route", "-n", "get", "default")
	if err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if string(output) != "gateway: 192.0.2.1\n" {
		t.Fatalf("Output() = %q", output)
	}
	if !outputRunner.called {
		t.Fatalf("delegate Output was not called")
	}
	if outputRunner.name != "route" || !reflect.DeepEqual(outputRunner.args, []string{"-n", "get", "default"}) {
		t.Fatalf("delegate command = %s %#v", outputRunner.name, outputRunner.args)
	}

	want := []string{"route -n get default"}
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
