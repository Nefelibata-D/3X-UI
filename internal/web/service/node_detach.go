package service

import (
	"slices"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
	"gorm.io/gorm"
)

// Let active remote syncs finish before forgetting their local records.
var nodeSyncLifecycle sync.RWMutex

func BeginNodeSync(id int) (func(), error) {
	nodeSyncLifecycle.RLock()
	var node model.Node
	if err := database.GetDB().Select("id").First(&node, id).Error; err != nil {
		nodeSyncLifecycle.RUnlock()
		return nil, err
	}
	return nodeSyncLifecycle.RUnlock, nil
}

// This only removes the master's copies; DelInbound would also delete remotely.
func detachNodeInbounds(tx *gorm.DB, nodeID int) error {
	var ids []int
	if err := tx.Model(&model.Inbound{}).Where("node_id = ?", nodeID).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("inbound_id IN ?", ids).Delete(&model.ClientInbound{}).Error; err != nil {
		return err
	}
	if _, err := database.DeleteHosts(tx, "inbound_id IN ?", ids); err != nil {
		return err
	}
	// Keep the client's accumulated usage, but discard its obsolete inbound hint.
	if err := tx.Model(&xray.ClientTraffic{}).Where("inbound_id IN ?", ids).Update("inbound_id", 0).Error; err != nil {
		return err
	}
	var balancers []model.SubBalancer
	if err := tx.Find(&balancers).Error; err != nil {
		return err
	}
	for i := range balancers {
		before := len(balancers[i].InboundIds)
		for _, id := range ids {
			delete(balancers[i].MemberWeights, id)
		}
		balancers[i].InboundIds = slices.DeleteFunc(balancers[i].InboundIds, func(id int) bool { return slices.Contains(ids, id) })
		if before != len(balancers[i].InboundIds) {
			if err := tx.Save(&balancers[i]).Error; err != nil {
				return err
			}
		}
	}
	return tx.Where("node_id = ?", nodeID).Delete(&model.Inbound{}).Error
}
