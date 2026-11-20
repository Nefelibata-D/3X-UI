package policy

import (
	"slices"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Resolve upstream identities here; node-ledger resets remain project-owned.
func RequestTrafficResetsByEmail(emails []string, batchSize int) error {
	var ids []int
	db := database.GetDB()
	for batch := range slices.Chunk(emails, batchSize) {
		var chunk []int
		if err := db.Model(&model.ClientRecord{}).Where("email IN ?", batch).Pluck("id", &chunk).Error; err != nil {
			return err
		}
		ids = append(ids, chunk...)
	}
	return RequestClientResets(ids)
}

func RequestAllTrafficResets() error {
	var ids []int
	if err := database.GetDB().Model(&model.ClientRecord{}).Pluck("id", &ids).Error; err != nil {
		return err
	}
	return RequestClientResets(ids)
}
