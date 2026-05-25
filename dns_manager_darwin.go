//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type DarwinDNSManager struct {
	Runner CommandRunner
}

func (m DarwinDNSManager) Apply(ctx context.Context, servers []string, cleanup *CleanupStack) error {
	out, err := m.Runner.Output(ctx, "networksetup", "-listallnetworkservices")
	if err != nil {
		return fmt.Errorf("list network services: %w", err)
	}

	services := parseDarwinNetworkServices(string(out))
	states := make([]darwinDNSState, 0, len(services))
	for _, service := range services {
		out, err := m.Runner.Output(ctx, "networksetup", "-getdnsservers", service)
		if err != nil {
			return fmt.Errorf("get DNS servers for service %s: %w", service, err)
		}
		states = append(states, parseDarwinDNSState(service, string(out)))
	}

	cleanup.Add("restore DNS servers", func() error {
		cleanupCtx := context.Background()
		var errs []error
		for _, state := range states {
			args := []string{"-setdnsservers", state.Service}
			if state.Empty {
				args = append(args, "Empty")
			} else {
				args = append(args, state.Servers...)
			}
			if err := m.Runner.Run(cleanupCtx, "networksetup", args...); err != nil {
				manual := "networksetup " + shellQuoteArgs(args)
				errs = append(errs, fmt.Errorf("restore DNS servers for service %s; manual recovery: %s: %w", state.Service, manual, err))
			}
		}
		return errors.Join(errs...)
	})

	for _, service := range services {
		args := append([]string{"-setdnsservers", service}, servers...)
		if err := m.Runner.Run(ctx, "networksetup", args...); err != nil {
			return fmt.Errorf("set DNS servers for service %s: %w", service, err)
		}
	}

	return nil
}

func shellQuoteArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, strconv.Quote(arg))
	}
	return strings.Join(quoted, " ")
}
