package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Resolve upstream identities here; node-ledger resets remain project-owned.
func requestClashTrafficResets(emails []string) error {
	var ids []int
	db := database.GetDB()
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var chunk []int
		if err := db.Model(&model.ClientRecord{}).Where("email IN ?", batch).Pluck("id", &chunk).Error; err != nil {
			return err
		}
		ids = append(ids, chunk...)
	}
	return policy.RequestClientResets(ids)
}

func requestAllClashTrafficResets() error {
	var ids []int
	if err := database.GetDB().Model(&model.ClientRecord{}).Pluck("id", &ids).Error; err != nil {
		return err
	}
	return policy.RequestClientResets(ids)
}
