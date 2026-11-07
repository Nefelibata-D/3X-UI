package subscription

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SubscriptionBinding struct {
	LastSyncAt        int64                    `json:"lastSyncAt" gorm:"->;default:0"`
	Dialers           map[string]string        `json:"dialers" gorm:"serializer:json"`
	UpdatedAt         int64                    `json:"updatedAt" gorm:"autoUpdateTime:milli"`
	SubID             string                   `json:"-" gorm:"primaryKey"`
	TemplateID        uint                     `json:"templateId"`
	DisabledRules     []string                 `json:"disabledRules" gorm:"serializer:json"`
	DisabledProviders []string                 `json:"disabledProviders" gorm:"serializer:json"`
	DisabledGroups    []string                 `json:"disabledGroups" gorm:"serializer:json"`
	Content           string                   `json:"content"`
	Groups            []TemplateGroupInjection `json:"groups" gorm:"serializer:json"`
}

func (SubscriptionBinding) TableName() string { return "clash_subscription_bindings" }

func subscriptionIdentity(clientID int) (string, error) {
	db, err := storage.DB()
	if err != nil {
		return "", err
	}
	var client model.ClientRecord
	if err := db.First(&client, clientID).Error; err != nil {
		return "", err
	}
	if client.SubID == "" {
		return "", errors.New("客户端没有订阅身份")
	}
	return client.SubID, nil
}

func ClientSubscription(clientID int) (SubscriptionBinding, error) {
	id, err := subscriptionIdentity(clientID)
	if err != nil {
		return SubscriptionBinding{}, err
	}
	row, err := subscriptionByID(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return SubscriptionBinding{Content: "{}", Groups: []TemplateGroupInjection{}}, nil
	}
	return row, err
}

func subscriptionByID(id string) (SubscriptionBinding, error) {
	db, err := storage.DB()
	if err != nil {
		return SubscriptionBinding{}, err
	}
	var row SubscriptionBinding
	err = db.Where("sub_id = ?", id).First(&row).Error
	return row, err
}

func SaveClientSubscription(clientID int, row *SubscriptionBinding) error {
	id, err := subscriptionIdentity(clientID)
	if err != nil {
		return err
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	row.SubID = id
	if row.TemplateID == 0 {
		return db.Delete(&SubscriptionBinding{}, "sub_id = ?", id).Error
	}
	var template YAMLTemplate
	if err := db.First(&template, row.TemplateID).Error; err != nil {
		return err
	}
	config, err := ComposeSubscription(template.Content, *row)
	if err != nil {
		return err
	}
	groups, err := subscriptionGroups(config)
	if err != nil {
		return err
	}
	if err := validateSubscriptionGraph(groups); err != nil {
		return err
	}
	seen := map[string]bool{}
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		return err
	}
	authorized := cat.AuthorizedNodeKeys(clientID)
	if client, ok := cat.ClientByID(clientID); ok {
		for _, id := range client.InboundIDs {
			if inbound, exists := cat.InboundByID(id); exists {
				if _, exists := cat.NodeByKey(inbound.NodeKey); exists {
					authorized = append(authorized, InboundKey(id))
				}
			}
		}
	}
	for _, binding := range row.Groups {
		if _, ok := groups[binding.Group]; !ok || seen[binding.Group] {
			return errors.New("注入代理组不存在或重复")
		}
		seen[binding.Group] = true
		if binding.AllNodes && len(binding.Nodes) > 0 {
			return errors.New("全部代理与指定代理不能同时选择")
		}
		for _, key := range binding.Nodes {
			if !slices.Contains(authorized, key) {
				return errors.New("注入代理不在客户端授权范围内")
			}
		}
	}
	row.UpdatedAt = time.Now().UnixMilli()
	return db.Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error
}

func RecordClientSubscriptionSync(subID, userAgent string) error {
	if !strings.Contains(strings.ToLower(userAgent), "clash-verge") {
		return nil
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	return db.Table((SubscriptionBinding{}).TableName()).Where("sub_id = ?", subID).UpdateColumn("last_sync_at", time.Now().UnixMilli()).Error
}

// InboundKey identifies the official inbound independently of its name or server.
