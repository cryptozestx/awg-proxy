//go:build darwin

package dns

import "strings"

type darwinDNSState struct {
	Service string
	Servers []string
	Empty   bool
}

func parseDarwinNetworkServices(out string) []string {
	services := make([]string, 0)
	for _, line := range strings.Split(out, "\n") {
		service := strings.TrimSpace(line)
		if service == "" || strings.HasPrefix(service, "An asterisk") || strings.HasPrefix(service, "*") {
			continue
		}
		services = append(services, service)
	}
	return services
}

func parseDarwinDNSState(service, out string) darwinDNSState {
	state := darwinDNSState{Service: service}
	if strings.TrimSpace(out) == "" || strings.Contains(out, "There aren't any DNS Servers") {
		state.Empty = true
		return state
	}
	state.Servers = strings.Fields(out)
	return state
}
