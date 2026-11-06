package web

import (
	"context"
	"errors"
	"fmt"
	"github.com/mhsanaei/3x-ui/v3/internal/clash"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// Start the gateway before Xray; report failures without taking the panel offline.
func (s *Server) startClash() {
	clash.Configure(clash.Hooks{
		SetInboundEnabled: func(tag string, enabled bool) error {
			db := database.GetDB()
			if db == nil {
				return errors.New("database is not initialized")
			}
			var ib model.Inbound
			if err := db.Where("tag = ?", tag).First(&ib).Error; err != nil {
				return err
			}
			if ib.NodeID != nil {
				return fmt.Errorf("inbound %q is not local", tag)
			}
			needRestart, err := (&service.InboundService{}).SetInboundEnable(ib.Id, enabled)
			if err != nil {
				return err
			}
			if needRestart {
				s.xrayService.SetToNeedRestart()
			}
			return nil
		},
		ApplyXray: func() error { return s.xrayService.RestartXray(false) },
	}, service.NewClashCatalogProvider())
	if err := clash.Start(context.Background()); err != nil {
		logger.Error("clash runtime start failed:", err)
	}
}
