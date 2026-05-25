package main

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

func (ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

var ErrDryRunOutputUnavailable = errors.New("dry-run output unavailable")

type DryRunRunner struct {
	commands     []string
	outputRunner CommandRunner
}

func NewDryRunRunner() *DryRunRunner {
	return &DryRunRunner{}
}

func NewDryRunRunnerWithOutput(outputRunner CommandRunner) *DryRunRunner {
	return &DryRunRunner{outputRunner: outputRunner}
}

func (r *DryRunRunner) Run(_ context.Context, name string, args ...string) error {
	r.commands = append(r.commands, commandString(name, args...))
	return nil
}

func (r *DryRunRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, commandString(name, args...))
	if r.outputRunner != nil {
		return r.outputRunner.Output(ctx, name, args...)
	}
	return nil, ErrDryRunOutputUnavailable
}

func (r *DryRunRunner) RecordDryRun(change string) {
	r.commands = append(r.commands, change)
}

func (r *DryRunRunner) Commands() []string {
	return append([]string(nil), r.commands...)
}

func commandString(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}
