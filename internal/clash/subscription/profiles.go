package subscription

import (
	"crypto/rand"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func defaultKeywords() []string {
	return []string{"FlClash/v{x.y.z}", "clash-verge/v{x.y.z.a}", "Shadowrocket"}
}

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
	var list []Profile
	err = db.Transaction(func(tx *gorm.DB) error {
		list, err = profiles(tx, owner)
		if err != nil {
			return err
		}
		for _, p := range list {
			if p.Token != nil {
				continue
			}
			token := rand.Text()
			row := p.SubscriptionBinding
			row.Owner, row.Token = owner, &token
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "sub_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			if err := tx.Model(&SubscriptionBinding{}).Where("sub_id = ? AND token IS NULL", row.SubID).UpdateColumns(map[string]any{"owner": owner, "token": token}).Error; err != nil {
				return err
			}
		}
		list, err = profiles(tx, owner)
		return err
	})
	return list, err
}

var shortToken = regexp.MustCompile(`^[a-zA-Z0-9]{20,32}$`)

type ProfileChange struct {
	Removed []struct {
		Key      string `json:"key"`
		Expected int64  `json:"expected"`
	} `json:"removed"`
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
			if !shortToken.MatchString(*row.Token) && (err != nil || id.Version() != 4 || id.String() != *row.Token) {
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
		for _, item := range change.Removed {
			index := slices.IndexFunc(list, func(p Profile) bool { return p.Key == item.Key })
			if index < 0 || item.Key == change.Key {
				return errors.New("待删除订阅已变更，请重新打开")
			}
			result := tx.Where("sub_id = ? AND COALESCE(updated_at,0) = ?", list[index].SubID, item.Expected).Delete(&SubscriptionBinding{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("待删除订阅已变更，请重新打开")
			}
		}
		saved = profileOf(owner, row)
		return nil
	})
	return saved, err
}

func ProfileByToken(token string) (SubscriptionBinding, error) {
	db, err := storage.DB()
	if err != nil {
		return SubscriptionBinding{}, err
	}
	var row SubscriptionBinding
	if !db.Migrator().HasTable(&SubscriptionBinding{}) {
		return row, gorm.ErrRecordNotFound
	}
	err = db.Where("token = ?", token).Take(&row).Error
	return row, err
}

// MatchClientProfile uses the same ordering as the subscription tabs.
func MatchClientProfile(owner, userAgent string) (*SubscriptionBinding, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	if !db.Migrator().HasTable(&SubscriptionBinding{}) {
		return nil, nil
	}
	list, err := profiles(db, owner)
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if slices.ContainsFunc(p.UAKeywords, func(word string) bool { return matchesUserAgent(word, userAgent) }) {
			if p.Key == "default" && p.Token == nil {
				return nil, nil
			}
			return &p.SubscriptionBinding, nil
		}
	}
	return nil, nil
}
