package main

import (
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(name string, args ...string) error
	Output(name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func (ExecRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

type DryRunRunner struct {
	commands []string
}

func NewDryRunRunner() *DryRunRunner {
	return &DryRunRunner{}
}

func (r *DryRunRunner) Run(name string, args ...string) error {
	r.commands = append(r.commands, commandString(name, args...))
	return nil
}

func (r *DryRunRunner) Output(name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, commandString(name, args...))
	return nil, nil
}

func (r *DryRunRunner) Commands() []string {
	return append([]string(nil), r.commands...)
}

func commandString(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), " ")
}
