//go:build linux

package tunnel

func defaultTunnelName() string {
	return "awgproxy0"
}
