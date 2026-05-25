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
	commands []string
}

func NewDryRunRunner() *DryRunRunner {
	return &DryRunRunner{}
}

func (r *DryRunRunner) Run(_ context.Context, name string, args ...string) error {
	r.commands = append(r.commands, commandString(name, args...))
	return nil
}

func (r *DryRunRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, commandString(name, args...))
	return nil, ErrDryRunOutputUnavailable
}

func (r *DryRunRunner) Commands() []string {
	return append([]string(nil), r.commands...)
}

func commandString(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}
