//go:build darwin

package tunnel

import "testing"

func TestDefaultTunnelNameDarwin(t *testing.T) {
	if got := defaultTunnelName(); got != "utun" {
		t.Fatalf("defaultTunnelName() = %q, want utun", got)
	}
}
