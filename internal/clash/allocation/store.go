package allocation

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ListNodeWeights() ([]NodeWeight, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var rows []NodeWeight
	err = db.Order("node_key ASC").Find(&rows).Error
	return rows, err
}

func UpsertNodeWeights(rows []NodeWeight) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	return db.Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			row := rows[i]
			if row.CreatedAt == 0 {
				row.CreatedAt = now
			}
			row.UpdatedAt = now
			if err := tx.Model(&NodeWeight{}).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "node_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"node_id", "node_guid", "default_weight", "capacity_bytes", "quota_reset_day", "quota_reset_minute", "quota_reset_disabled", "updated_at"}),
			}).Create(map[string]any{
				"node_key": row.NodeKey, "node_id": row.NodeID, "node_guid": row.NodeGuid,
				"default_weight": row.DefaultWeight,
				"capacity_bytes": row.CapacityBytes, "quota_reset_day": row.ResetSchedule.Day, "quota_reset_minute": row.ResetSchedule.Minute, "quota_reset_disabled": row.ResetSchedule.Disabled,
				"created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ListRatios() ([]ClientRatio, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var rows []ClientRatio
	err = db.Order("client_id ASC").Find(&rows).Error
	return rows, err
}

func GetRatio(clientID int) (*ClientRatio, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var row ClientRatio
	if err := db.Where("client_id = ?", clientID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func UpsertRatio(row ClientRatio) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if row.CreatedAt == 0 {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"email", "mode", "weights_json", "limits_json", "reset_mode", "updated_at"}),
	}).Create(&row).Error
}
