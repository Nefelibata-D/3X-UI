package subscription

import (
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func defaultKeywords() []string { return []string{"clash-verge", "ShadowRocket"} }

type Profile struct {
	Key string `json:"key"`
	SubscriptionBinding
}

func profileOf(owner string, row SubscriptionBinding) Profile {
	key := strings.TrimPrefix(row.SubID, "/profile/")
	if row.SubID == owner {
		key = "default"
	}
	if row.Name == "" {
		row.Name = "默认订阅"
	}
	if row.UAKeywords == nil {
		row.UAKeywords = defaultKeywords()
	}
	return Profile{key, row}
}

func profiles(db *gorm.DB, owner string) ([]Profile, error) {
	var rows []SubscriptionBinding
	if err := db.Where("owner = ? OR sub_id = ?", owner, owner).Order("sub_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		rows = append(rows, SubscriptionBinding{SubID: owner, Content: "{}"})
	}
	result := make([]Profile, 0, len(rows))
	for _, row := range rows {
		profile := profileOf(owner, row)
		if profile.Key == "default" {
			result = append([]Profile{profile}, result...)
		} else {
			result = append(result, profile)
		}
	}
	return result, nil
}

func ClientProfiles(clientID int) ([]Profile, error) {
	owner, err := subscriptionIdentity(clientID)
	if err != nil {
		return nil, err
	}
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	return profiles(db, owner)
}

type ProfileChange struct {
	Profile
	Create   bool  `json:"create"`
	Delete   bool  `json:"delete"`
	Expected int64 `json:"expected"`
}

func ChangeProfile(clientID int, change ProfileChange) (Profile, error) {
	db, err := storage.DB()
	if err != nil {
		return Profile{}, err
	}
	var saved Profile
	err = db.Transaction(func(tx *gorm.DB) error {
		var client model.ClientRecord
		if err := tx.Select("id", "sub_id").First(&client, clientID).Error; err != nil {
			return err
		}
		owner := client.SubID
		if owner == "" {
			return errors.New("客户端没有订阅身份")
		}
		var owners []int
		if err := tx.Model(&model.ClientRecord{}).Clauses(clause.Locking{Strength: "UPDATE"}).Where("sub_id = ?", owner).Order("id").Pluck("id", &owners).Error; err != nil {
			return err
		}
		if !slices.Contains(owners, clientID) {
			return errors.New("客户端订阅身份已变更")
		}
		list, err := profiles(tx, owner)
		if err != nil {
			return err
		}
		var old *Profile
		for i := range list {
			if list[i].Key == change.Key {
				old = &list[i]
				break
			}
		}
		row := change.SubscriptionBinding
		if change.Create {
			if old != nil || change.Delete {
				return errors.New("订阅已存在")
			}
			id, err := uuid.Parse(change.Key)
			if err != nil || id.Version() != 4 || id.String() != change.Key {
				return errors.New("无效的订阅编号")
			}
			row.SubID = "/profile/" + id.String()
			if len(list) == 1 && list[0].Key == "default" {
				initial := list[0].SubscriptionBinding
				if err := tx.Omit("UpdatedAt").Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
					return err
				}
			}
		} else {
			if old == nil || old.UpdatedAt != change.Expected {
				return errors.New("订阅已变更，请重新打开")
			}
			row.SubID = old.SubID
			if change.Delete {
				if len(list) <= 1 {
					return errors.New("至少保留一个订阅")
				}
				return tx.Delete(&SubscriptionBinding{}, "sub_id = ?", row.SubID).Error
			}
		}
		row.Owner = owner
		row.Name = strings.TrimSpace(row.Name)
		if row.Name == "" {
			return errors.New("订阅名称不能为空")
		}
		if row.Token == nil && (change.Create || old.Token != nil) {
			return errors.New("订阅链接无效")
		}
		if row.Token != nil {
			id, err := uuid.Parse(*row.Token)
			if err != nil || id.Version() != 4 || id.String() != *row.Token {
				return errors.New("订阅链接无效")
			}
		}
		for _, w := range row.UAKeywords {
			if strings.TrimSpace(w) == "" {
				return errors.New("UA 关键词不能为空")
			}
		}
		if err := writeBinding(tx, clientID, &row, change.Create, change.Expected); err != nil {
			return err
		}
		if old != nil {
			row.LastSyncAt = old.LastSyncAt
		}
		saved = profileOf(owner, row)
		return nil
	})
	return saved, err
}

// Existing URLs stay valid until their default profile is rotated or removed.
func DefaultLinkActive(owner string) (bool, error) {
	db, err := storage.DB()
	if err != nil {
		return false, err
	}
	if !db.Migrator().HasTable(&SubscriptionBinding{}) {
		return true, nil
	}
	var n int64
	err = db.Model(&SubscriptionBinding{}).Where("owner = ?", owner).Count(&n).Error
	if err != nil || n == 0 {
		return err == nil, err
	}
	err = db.Model(&SubscriptionBinding{}).Where("sub_id = ? AND token IS NULL", owner).Count(&n).Error
	return n != 0, err
}

func ProfileByToken(token string) (SubscriptionBinding, error) {
	db, err := storage.DB()
	if err != nil {
		return SubscriptionBinding{}, err
	}
	var row SubscriptionBinding
	err = db.Where("token = ?", token).Take(&row).Error
	return row, err
}
