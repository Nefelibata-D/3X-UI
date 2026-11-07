package subscription

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/allocation"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

type subscriptionPublication struct{ subID, baseURL string }

// RenderManagedSubscription shares the final renderer with validation and subscription downloads.
func RenderManagedSubscription(subID string, draft *SubscriptionBinding, proxies []map[string]any, nodeKeys, inboundKeys map[string]string, baseURL string) (map[string]any, bool, error) {
	var binding SubscriptionBinding
	if draft != nil {
		binding = *draft
	} else {
		db, err := storage.DB()
		if err != nil {
			return nil, false, err
		}
		if !db.Migrator().HasTable(&SubscriptionBinding{}) {
			return nil, false, nil
		}
		binding, err = subscriptionByID(subID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, true, err
		}
	}
	return renderSubscriptionBinding(binding, proxies, nodeKeys, &subscriptionPublication{subID, baseURL}, inboundKeys)
}

func (p *subscriptionPublication) inject(config map[string]any, nodeKeys map[string]string) error {
	groups, err := subscriptionGroups(config)
	if err != nil {
		return err
	}
	weighted := false
	for _, group := range groups {
		if group["type"] == "load-balance" {
			if err := validateControlURL(p.baseURL); err != nil {
				return err
			}
			setSubscriptionHealth(group, p.baseURL)
		}
		if group["type"] == "load-balance" && group["strategy"] == "weighted-sticky" {
			weighted = true
		}
	}
	if !weighted {
		return nil
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	var clients []model.ClientRecord
	if err := db.Where("sub_id = ?", p.subID).Find(&clients).Error; err != nil {
		return err
	}
	if len(clients) != 1 {
		return errors.New("加权订阅必须对应唯一的官方用户，不能共用多个用户的订阅身份")
	}
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		return err
	}
	client, ok := cat.ClientByID(clients[0].Id)
	if !ok {
		return errors.New("订阅用户不在官方目录中")
	}
	rows, err := allocation.ListNodeWeights()
	if err != nil {
		return err
	}
	ratio, err := allocation.GetRatio(client.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	weights, err := allocation.EffectiveWeights(cat, client.ID, allocation.NodeConfigMap(cat, rows), ratio)
	if err != nil {
		return err
	}
	return InjectSubscriptionPolicy(config, cat, client, weights, nodeKeys, p.baseURL)
}

// InjectSubscriptionPolicy owns generated identity and weights; templates retain tuning parameters.
func InjectSubscriptionPolicy(config map[string]any, cat *catalog.Catalog, client catalog.ClientInfo, weights []allocation.RatioResult, nodeKeys map[string]string, baseURL string) error {
	if err := validateControlURL(baseURL); err != nil {
		return err
	}
	if !validControlUUID(client.UUID) {
		return errors.New("加权订阅用户缺少有效的官方 UUID")
	}
	groups, err := subscriptionGroups(config)
	if err != nil {
		return err
	}
	authorized := map[string]bool{}
	for _, key := range cat.AuthorizedNodeKeys(client.ID) {
		authorized[key] = true
	}
	byNode := map[string]int64{}
	for _, weight := range weights {
		byNode[weight.Key] = weight.Weight
	}
	for name, group := range groups {
		if group["type"] != "load-balance" || group["strategy"] != "weighted-sticky" {
			continue
		}
		if use, _ := group["use"].([]any); len(use) > 0 || group["include-all"] == true || group["include-all-proxies"] == true || group["include-all-providers"] == true {
			return fmt.Errorf("加权组 %s 必须显式选择受管代理，不能使用动态 provider 或 include-all", name)
		}
		opts := map[string]any{}
		if raw, exists := group["weighted-sticky"]; exists {
			var ok bool
			opts, ok = raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s weighted-sticky 必须是对象", name)
			}
		}
		members, _ := group["proxies"].([]any)
		generatedWeights, ids := map[string]any{}, map[string]any{}
		for _, raw := range members {
			proxy, ok := raw.(string)
			if !ok {
				return fmt.Errorf("%s 的成员必须是代理名称", name)
			}
			key := nodeKeys[proxy]
			node, exists := cat.NodeByKey(key)
			if !exists || !authorized[key] || strings.TrimSpace(node.Guid) == "" {
				return fmt.Errorf("加权组 %s 的成员 %s 没有授权的面板身份；请选择具体受管代理", name, proxy)
			}
			weight, selectable := byNode[key]
			if !selectable {
				return fmt.Errorf("加权组 %s 的成员 %s 所属节点不可用", name, proxy)
			}
			generatedWeights[proxy], ids[proxy] = weight, node.Guid
		}
		opts["weights"] = generatedWeights
		opts["default-weight"] = 0
		resilience := map[string]any{}
		if raw, exists := opts["resilience"]; exists {
			var ok bool
			resilience, ok = raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s resilience 必须是对象", name)
			}
		}
		control := map[string]any{}
		if raw, exists := resilience["control"]; exists {
			var ok bool
			control, ok = raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s control 必须是对象", name)
			}
		}
		control["base-url"], control["uuid"], control["node-ids"] = strings.TrimRight(baseURL, "/"), client.UUID, ids
		for key, value := range map[string]any{"timeout": 3, "min-interval": 30, "grace-interval": 300} {
			if _, exists := control[key]; !exists {
				control[key] = value
			}
		}
		resilience["enabled"], resilience["control"] = true, control
		opts["resilience"] = resilience
		for key, value := range map[string]any{"idle-timeout": 300, "max-bindings": 10000} {
			if _, exists := opts[key]; !exists {
				opts[key] = value
			}
		}
		group["weighted-sticky"] = opts
		setSubscriptionHealth(group, baseURL)
	}
	return ValidateWeightedSubscription(config)
}

func setSubscriptionHealth(group map[string]any, baseURL string) {
	if _, exists := group["url"]; !exists {
		group["url"], group["expected-status"] = strings.TrimRight(baseURL, "/")+"/health", 204
	}
	if _, exists := group["interval"]; !exists {
		group["interval"] = 300
	}
	if _, exists := group["lazy"]; !exists {
		group["lazy"] = false
	}
}
