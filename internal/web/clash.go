package web

import (
	"context"

	"github.com/mhsanaei/3x-ui/v3/internal/clash"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/integration"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// Start the gateway before Xray; report failures without taking the panel offline.
func (s *Server) startClash() {
	policy.Configure(policy.Hooks{
		ApplyXray: func() error { return s.xrayService.RestartXray(false) },
	}, integration.LoadCatalog, clash.Migrate)
	if err := clash.Start(context.Background()); err != nil {
		logger.Error("clash runtime start failed:", err)
	}
}
