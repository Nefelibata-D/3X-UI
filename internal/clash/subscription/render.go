package subscription

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"gorm.io/gorm"
)

func RenderSubscription(subID string, proxies []map[string]any, nodeKeys map[string]string, inboundKeys ...map[string]string) (map[string]any, bool, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, false, err
	}
	if !db.Migrator().HasTable(&SubscriptionBinding{}) {
		return nil, false, nil
	}
	binding, err := subscriptionByID(subID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	return RenderSubscriptionBinding(binding, proxies, nodeKeys, inboundKeys...)
}

func RenderSubscriptionBinding(binding SubscriptionBinding, proxies []map[string]any, nodeKeys map[string]string, inboundKeys ...map[string]string) (map[string]any, bool, error) {
	return renderSubscriptionBinding(binding, proxies, nodeKeys, nil, inboundKeys...)
}

func renderSubscriptionBinding(binding SubscriptionBinding, proxies []map[string]any, nodeKeys map[string]string, publication *subscriptionPublication, inboundKeys ...map[string]string) (map[string]any, bool, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, true, err
	}
	var template YAMLTemplate
	if err := db.First(&template, binding.TemplateID).Error; err != nil {
		return nil, true, err
	}
	config, err := ComposeSubscription(template.Content, binding)
	if err != nil {
		return nil, true, err
	}
	groups, err := subscriptionGroups(config)
	if err != nil {
		return nil, true, err
	}
	selected := []map[string]any{}
	for _, proxy := range proxies {
		name, _ := proxy["name"].(string)
		include := false
		for _, injection := range binding.Groups {
			if injectionIncludes(injection, name, nodeKeys, inboundKeys) {
				include = true
			}
		}
		for source, target := range binding.Dialers {
			if target != "" && (name == source || name == target) {
				include = true
			}
		}
		if include {
			selected = append(selected, proxy)
		}
	}
	all := []map[string]any{}
	if static, ok := config["proxies"].([]any); ok {
		for _, value := range static {
			proxy, ok := value.(map[string]any)
			if !ok {
				return nil, true, errors.New("模板代理配置必须是对象")
			}
			all = append(all, proxy)
		}
	}
	all = append(all, selected...)
	config["proxies"] = all
	for _, injection := range binding.Groups {
		group := groups[injection.Group]
		if group == nil {
			return nil, true, fmt.Errorf("注入代理组不存在：%s", injection.Group)
		}
		members, _ := group["proxies"].([]any)
		for _, proxy := range proxies {
			name, _ := proxy["name"].(string)
			if injectionIncludes(injection, name, nodeKeys, inboundKeys) {
				found := false
				for _, member := range members {
					if member == name {
						found = true
						break
					}
				}
				if !found {
					members = append(members, name)
				}
			}
		}
		group["proxies"] = members
	}
	for name, group := range groups {
		members, _ := group["proxies"].([]any)
		use, _ := group["use"].([]any)
		if len(members) == 0 && len(use) == 0 && group["include-all"] != true && group["include-all-proxies"] != true && group["include-all-providers"] != true {
			return nil, true, fmt.Errorf("节点组 %s 没有成员，请配置注入", name)
		}
	}
	if err := validateSubscriptionGraph(groups); err != nil {
		return nil, true, err
	}
	if publication != nil {
		if err := publication.inject(config, nodeKeys); err != nil {
			return nil, true, err
		}
	}
	if err := ValidatePublishedSubscription(config, all, binding.Dialers); err != nil {
		return nil, true, err
	}
	return config, true, nil
}

func InboundKey(id int) string { return fmt.Sprintf("inbound:%d", id) }

// Server keys remain readable for existing bindings; new editors select inbound keys.
func injectionIncludes(injection TemplateGroupInjection, name string, nodeKeys map[string]string, inboundKeys []map[string]string) bool {
	if injection.AllNodes || slices.Contains(injection.Nodes, nodeKeys[name]) {
		return true
	}
	return len(inboundKeys) > 0 && inboundKeys[0][name] != "" && slices.Contains(injection.Nodes, inboundKeys[0][name])
}
