//go:build darwin

package tunnel

func defaultTunnelName() string {
	return "utun"
}
