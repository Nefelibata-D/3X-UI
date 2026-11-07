package web

import (
	"context"

	"github.com/mhsanaei/3x-ui/v3/internal/clash"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// Start the gateway before Xray; report failures without taking the panel offline.
func (s *Server) startClash() {
	clash.Configure(clash.Hooks{
		ApplyXray: func() error { return s.xrayService.RestartXray(false) },
	}, service.NewClashCatalogProvider())
	if err := clash.Start(context.Background()); err != nil {
		logger.Error("clash runtime start failed:", err)
	}
}
