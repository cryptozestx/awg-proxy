package app

import (
	"awg-proxy/internal/config"
	"awg-proxy/internal/tunnel"
)

func (r Runtime) runTunnelMode(cfg *config.AWGConfig, opts Options) error {
	return tunnel.Run(cfg, opts.Tunnel)
}
