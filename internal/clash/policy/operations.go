package policy

import (
	"context"
	"errors"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"gorm.io/gorm"
)

// Keep IN queries below SQLite's parameter limit, including the state predicates.
const resetClientBatchSize = 400

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
	return RequestClientResets([]int{clientID})
}

// RequestClientResets durably queues one reset per client/node ledger.
// Repeated requests while delivery is pending keep the same reset revision.
func RequestClientResets(clientIDs []int) error {
	if len(clientIDs) == 0 {
		return nil
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	rt := GetRuntime()
	if rt != nil {
		rt.syncMu.Lock()
		defer rt.syncMu.Unlock()
	}
	var changed int64
	err = db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(clientIDs); start += resetClientBatchSize {
			end := min(start+resetClientBatchSize, len(clientIDs))
			result := tx.Model(&Delivery{}).Where("client_id IN ? AND remove = ? AND reset_pending = ?", clientIDs[start:end], false, false).
				Updates(map[string]any{"revision": gorm.Expr("revision + 1"), "reset_pending": true, "status": DeliveryPending, "last_error": ""})
			if result.Error != nil {
				return result.Error
			}
			changed += result.RowsAffected
		}
		return nil
	})
	if err != nil {
		return err
	}
	if changed > 0 {
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

func LoadCatalogOrError() (*catalog.Catalog, error) {
	rt := GetRuntime()
	if rt == nil {
		return nil, errors.New("clash runtime is not configured")
	}
	return rt.catalogNow()
}
