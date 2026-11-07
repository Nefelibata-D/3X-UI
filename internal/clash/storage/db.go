package storage

import (
	"errors"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"gorm.io/gorm"
)

func DB() (*gorm.DB, error) {
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("clash: database is not initialized")
	}
	return db, nil
}
