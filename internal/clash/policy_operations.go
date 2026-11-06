package clash

import (
	"context"
	"errors"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"gorm.io/gorm"
)

func RequestReset(nodeKey string, clientID int) error {
	rt := GetRuntime()
	if rt == nil {
		return errors.New("clash runtime is not configured")
	}
	rt.syncMu.Lock()
	defer rt.syncMu.Unlock()
	row, err := GetDelivery(nodeKey, clientID)
	if err != nil {
		return err
	}
	if !row.ResetPending {
		row.Revision++
	}
	row.ResetPending = true
	row.Status = DeliveryPending
	row.LastError = ""
	row.Remove = false
	if err := UpsertDelivery(*row); err != nil {
		return err
	}
	TriggerSync()
	return nil
}

func RequestClientReset(clientID int) error {
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	rt := GetRuntime()
	if rt != nil {
		rt.syncMu.Lock()
		defer rt.syncMu.Unlock()
	}
	result := db.Model(&Delivery{}).Where("client_id = ? AND remove = ? AND reset_pending = ?", clientID, false, false).
		Updates(map[string]any{"revision": gorm.Expr("revision + 1"), "reset_pending": true, "status": DeliveryPending, "last_error": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		TriggerSync()
	}
	return nil
}

func TriggerSync() {
	rt := GetRuntime()
	if rt == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Warningf("clash: background sync panic: %v", r)
			}
		}()
		rt.SyncOnce(context.Background())
	}()
}

func LoadCatalogOrError() (*Catalog, error) {
	rt := GetRuntime()
	if rt == nil {
		return nil, errors.New("clash runtime is not configured")
	}
	return rt.catalogNow()
}
