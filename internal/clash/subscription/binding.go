package subscription

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

type SubscriptionBinding struct {
	Owner      string   `json:"-" gorm:"index"`
	Name       string   `json:"name,omitempty"`
	Token      *string  `json:"token,omitempty" gorm:"uniqueIndex"`
	UAKeywords []string `json:"uaKeywords" gorm:"serializer:json"`

	LastSyncAt        int64                    `json:"lastSyncAt" gorm:"->;default:0"`
	Dialers           map[string]string        `json:"dialers" gorm:"serializer:json"`
	UpdatedAt         int64                    `json:"updatedAt" gorm:"autoUpdateTime:milli"`
	SubID             string                   `json:"-" gorm:"primaryKey"`
	TemplateID        uint                     `json:"templateId"`
	RuleOrder         []string                 `json:"ruleOrder" gorm:"serializer:json"`
	GroupOrder        []string                 `json:"groupOrder" gorm:"serializer:json"`
	ProviderOrder     []string                 `json:"providerOrder" gorm:"serializer:json"`
	PausedContent     string                   `json:"pausedContent"`
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
	return db.Transaction(func(tx *gorm.DB) error {
		var old SubscriptionBinding
		if err := tx.Where("sub_id = ?", id).Take(&old).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row.SubID = id
		row.Name, row.Token, row.Owner, row.UAKeywords = old.Name, old.Token, old.Owner, old.UAKeywords
		if row.TemplateID == 0 && old.Owner == "" && old.Name == "" {
			return tx.Delete(&SubscriptionBinding{}, "sub_id = ?", id).Error
		}
		return writeBinding(tx, clientID, row, false, old.UpdatedAt)
	})
}
func writeBinding(db *gorm.DB, clientID int, row *SubscriptionBinding, create bool, previous int64) error {
	if row.TemplateID != 0 {
		if err := validateBinding(db, clientID, row); err != nil {
			return err
		}
	}
	row.UpdatedAt = max(time.Now().UnixMilli(), previous+1)
	if create {
		return db.Create(row).Error
	}
	result := db.Model(row).Where("sub_id = ? AND COALESCE(updated_at,0) = ?", row.SubID, previous).Select("*").Omit("SubID", "LastSyncAt").UpdateColumns(row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 0 {
		return nil
	}
	if previous != 0 {
		return errors.New("订阅已变更，请重新打开")
	}
	return db.Create(row).Error
}
func validateBinding(db *gorm.DB, clientID int, row *SubscriptionBinding) error {
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
	return nil
}

func matchesUserAgent(pattern, userAgent string) bool {
	if pattern == "" {
		return false
	}
	expression := regexp.QuoteMeta(strings.ToLower(pattern))
	expression = strings.ReplaceAll(expression, regexp.QuoteMeta("{x.y.z}"), `[0-9]+\.[0-9]+\.[0-9]+`)
	expression = strings.ReplaceAll(expression, regexp.QuoteMeta("{x.y.z.a}"), `[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+`)
	matched, _ := regexp.MatchString("^"+expression, strings.ToLower(userAgent))
	return matched
}

func RecordClientSubscriptionSync(subID, userAgent string, token *string) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	row, err := subscriptionByID(subID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(row.UAKeywords, func(w string) bool { return matchesUserAgent(w, userAgent) }) {
		return nil
	}
	query := db.Table((SubscriptionBinding{}).TableName()).Where("sub_id = ?", subID)
	if token == nil {
		query = query.Where("token IS NULL")
	} else {
		query = query.Where("token = ?", *token)
	}
	return query.UpdateColumn("last_sync_at", time.Now().UnixMilli()).Error
}
