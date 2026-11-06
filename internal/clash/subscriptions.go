package clash

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
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
	db, err := dbOrErr()
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
	db, err := dbOrErr()
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
	db, err := dbOrErr()
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
	cat, err := LoadCatalogOrError()
	if err != nil {
		return err
	}
	authorized := cat.AuthorizedNodeKeys(clientID)
	for _, binding := range row.Groups {
		if _, ok := groups[binding.Group]; !ok || seen[binding.Group] {
			return errors.New("注入节点组不存在或重复")
		}
		seen[binding.Group] = true
		if binding.AllNodes && len(binding.Nodes) > 0 {
			return errors.New("全部节点与指定节点不能同时选择")
		}
		for _, key := range binding.Nodes {
			if !slices.Contains(authorized, key) {
				return errors.New("注入节点不在客户端授权范围内")
			}
		}
	}
	row.UpdatedAt = nowMillis()
	return db.Clauses(clause.OnConflict{UpdateAll: true}).Create(row).Error
}

// RuleIdentity excludes the target and optional flags; logical expressions retain their full predicate.
func RuleIdentity(rule string) (string, error) {
	first := strings.IndexByte(rule, ',')
	if first <= 0 {
		return "", errors.New("规则缺少类型或目标")
	}
	kind := strings.ToUpper(strings.TrimSpace(rule[:first]))
	remainder := strings.TrimSpace(rule[first+1:])
	if kind == "MATCH" {
		if remainder == "" {
			return "", errors.New("规则缺少目标")
		}
		return kind, nil
	}
	for {
		last := strings.LastIndexByte(remainder, ',')
		if last < 0 {
			return "", errors.New("规则缺少匹配内容或目标")
		}
		tail := strings.TrimSpace(remainder[last+1:])
		if strings.EqualFold(tail, "no-resolve") || strings.EqualFold(tail, "src") {
			remainder = remainder[:last]
			continue
		}
		predicate := strings.TrimSpace(remainder[:last])
		if predicate == "" || tail == "" {
			return "", errors.New("规则缺少匹配内容或目标")
		}
		return kind + "," + predicate, nil
	}
}

func ComposeSubscription(base string, binding SubscriptionBinding) (map[string]any, error) {
	var config, overlay map[string]any
	if err := yaml.Unmarshal([]byte(base), &config); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal([]byte(binding.Content), &overlay); err != nil {
		return nil, err
	}
	if config == nil || overlay == nil {
		return nil, errors.New("订阅配置必须是 YAML 对象")
	}
	for key := range overlay {
		if key == "proxies" {
			return nil, errors.New("代理凭据由客户端授权节点生成，不能直接覆盖")
		}
		if key != "rules" && key != "rule-providers" && key != "proxy-groups" {
			if overlay[key] == nil {
				delete(config, key)
			} else {
				config[key] = overlay[key]
			}
		}
	}
	if blacklist, exists := overlay["site-blacklist"]; exists {
		if _, ok := blacklist.([]any); blacklist != nil && !ok {
			return nil, errors.New("黑名单必须是列表")
		}
		if blacklist != nil {
			config["site-blacklist"] = blacklist
		}
	}
	for _, key := range []string{"rules", "proxy-groups"} {
		original, ok := config[key].([]any)
		if config[key] != nil && !ok {
			return nil, fmt.Errorf("%s 必须是列表", key)
		}
		added, ok := overlay[key].([]any)
		if overlay[key] != nil && !ok {
			return nil, fmt.Errorf("%s 必须是列表", key)
		}
		identity := func(item any) (string, error) {
			if key == "rules" {
				s, ok := item.(string)
				if !ok {
					return "", errors.New("规则必须是字符串")
				}
				return RuleIdentity(s)
			}
			m, ok := item.(map[string]any)
			if !ok {
				return "", errors.New("节点组必须是对象")
			}
			name, _ := m["name"].(string)
			if name == "" {
				return "", errors.New("节点组名称不能为空")
			}
			return name, nil
		}
		disabled := binding.DisabledRules
		if key == "proxy-groups" {
			disabled = binding.DisabledGroups
		}
		replacements := map[string]any{}
		used := map[string]bool{}
		for _, item := range added {
			id, err := identity(item)
			if err != nil {
				return nil, err
			}
			if _, exists := replacements[id]; exists {
				return nil, fmt.Errorf("重复覆盖项 %s", id)
			}
			replacements[id] = item
		}
		out := []any{}
		for _, item := range original {
			id, err := identity(item)
			if err != nil {
				return nil, err
			}
			if replacement, exists := replacements[id]; exists {
				if !used[id] {
					out = append(out, replacement)
					used[id] = true
				}
				continue
			}
			if !slices.Contains(disabled, id) {
				out = append(out, item)
			}
		}
		additional := []any{}
		for _, item := range added {
			id, _ := identity(item)
			if !used[id] {
				additional = append(additional, item)
			}
		}
		if key == "rules" {
			index := len(out)
			for i, item := range out {
				id, _ := identity(item)
				if id == "MATCH" {
					index = i
					break
				}
			}
			out = append(append(append([]any{}, out[:index]...), additional...), out[index:]...)
		} else {
			out = append(out, additional...)
		}
		config[key] = out
	}
	providers, ok := config["rule-providers"].(map[string]any)
	if config["rule-providers"] != nil && !ok {
		return nil, errors.New("规则集必须是对象")
	}
	if providers == nil {
		providers = map[string]any{}
	}
	for _, name := range binding.DisabledProviders {
		delete(providers, name)
	}
	added, ok := overlay["rule-providers"].(map[string]any)
	if overlay["rule-providers"] != nil && !ok {
		return nil, errors.New("规则集必须是对象")
	}
	for name, value := range added {
		providers[name] = value
	}
	if len(providers) > 0 {
		config["rule-providers"] = providers
	} else {
		delete(config, "rule-providers")
	}
	return config, nil
}

func subscriptionGroups(config map[string]any) (map[string]map[string]any, error) {
	groups := map[string]map[string]any{}
	list, _ := config["proxy-groups"].([]any)
	for _, item := range list {
		group, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("无效节点组")
		}
		name, _ := group["name"].(string)
		if name == "" || groups[name] != nil {
			return nil, errors.New("节点组名称为空或重复")
		}
		groups[name] = group
	}
	return groups, nil
}

func validateSubscriptionGraph(groups map[string]map[string]any) error {
	active, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if active[name] {
			return fmt.Errorf("节点组循环引用：%s", name)
		}
		if done[name] {
			return nil
		}
		active[name] = true
		group := groups[name]
		members, ok := group["proxies"].([]any)
		if group["proxies"] != nil && !ok {
			return fmt.Errorf("节点组 %s 的成员必须是列表", name)
		}
		for _, value := range members {
			member, ok := value.(string)
			if !ok {
				return fmt.Errorf("节点组 %s 的成员必须是名称", name)
			}
			if groups[member] != nil {
				if err := visit(member); err != nil {
					return err
				}
			}
		}
		active[name] = false
		done[name] = true
		return nil
	}
	for name := range groups {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func RenderSubscription(subID string, proxies []map[string]any, nodeKeys map[string]string) (map[string]any, bool, error) {
	db, err := dbOrErr()
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
	return RenderSubscriptionBinding(binding, proxies, nodeKeys)
}

func RenderSubscriptionBinding(binding SubscriptionBinding, proxies []map[string]any, nodeKeys map[string]string) (map[string]any, bool, error) {
	db, err := dbOrErr()
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
			if injection.AllNodes || slices.Contains(injection.Nodes, nodeKeys[name]) {
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
			return nil, true, fmt.Errorf("注入节点组不存在：%s", injection.Group)
		}
		members, _ := group["proxies"].([]any)
		for _, proxy := range proxies {
			name, _ := proxy["name"].(string)
			if injection.AllNodes || slices.Contains(injection.Nodes, nodeKeys[name]) {
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
	if err := ValidatePublishedSubscription(config, all, binding.Dialers); err != nil {
		return nil, true, err
	}
	return config, true, nil
}

func RecordClientSubscriptionSync(subID, userAgent string) error {
	if !strings.Contains(strings.ToLower(userAgent), "clash-verge") {
		return nil
	}
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	return db.Table((SubscriptionBinding{}).TableName()).Where("sub_id = ?", subID).UpdateColumn("last_sync_at", time.Now().UnixMilli()).Error
}
