package subscription

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/idna"
)

func validControlUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }
func validateControlURL(value string) error {
	u, err := url.Parse(value)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("control base-url 必须是无凭据、查询参数和片段的 HTTP(S) 地址")
	}
	return nil
}
func weightedInteger(value any, minimum, maximum int64) (int64, error) {
	var number int64
	v := reflect.ValueOf(value)
	if !v.IsValid() {
		return 0, fmt.Errorf("必须是整数")
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number = v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v.Uint() > uint64(maximum) {
			return 0, fmt.Errorf("整数超出范围")
		}
		number = int64(v.Uint())
	case reflect.Float32, reflect.Float64:
		n := v.Float()
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) || n >= float64(math.MaxInt64) {
			return 0, fmt.Errorf("必须是有限整数")
		}
		number = int64(n)
	default:
		return 0, fmt.Errorf("必须是整数，不能使用字符串或布尔值")
	}
	if number < minimum || number > maximum {
		return 0, fmt.Errorf("整数超出范围")
	}
	return number, nil
}
func weightedKeys(object map[string]any, keys ...string) error {
	for key := range object {
		found := false
		for _, allowed := range keys {
			if key == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("未知加权配置字段：%s", key)
		}
	}
	return nil
}

// ValidateWeightedSubscription checks the published wire contract, after generated members and identity exist.
func ValidateWeightedSubscription(config map[string]any) error {
	groups, err := subscriptionGroups(config)
	if err != nil {
		return err
	}
	for name, group := range groups {
		raw, exists := group["weighted-sticky"]
		if !exists {
			continue
		}
		if group["type"] != "load-balance" || group["strategy"] != "weighted-sticky" {
			return fmt.Errorf("%s 的 weighted-sticky 配置要求同名负载策略", name)
		}
		opts, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s weighted-sticky 必须是对象", name)
		}
		if err := weightedKeys(opts, "idle-timeout", "nodes", "max-bindings", "site-groups", "resilience"); err != nil {
			return err
		}
		if value, exists := opts["site-groups"]; exists {
			if err := validateSiteGroups(value); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		for _, key := range []string{"idle-timeout", "max-bindings"} {
			if value, exists := opts[key]; exists {
				max := int64(math.MaxInt64)
				if key == "idle-timeout" {
					max /= 1000000000
				}
				if _, err := weightedInteger(value, 1, max); err != nil {
					return fmt.Errorf("%s %s：%w", name, key, err)
				}
			}
		}
		members, _ := group["proxies"].([]any)
		known := map[string]bool{}
		for _, value := range members {
			if member, ok := value.(string); ok {
				if groups[member] != nil {
					return fmt.Errorf("加权组 %s 不支持嵌套代理组 %s", name, member)
				}
				if known[member] {
					return fmt.Errorf("%s: duplicate proxy %s", name, member)
				}
				known[member] = true
			}
		}
		use, _ := group["use"].([]any)
		dynamic := len(use) > 0 || group["include-all"] == true || group["include-all-proxies"] == true || group["include-all-providers"] == true
		if err := validateWeightedNodes(opts["nodes"], known, dynamic); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if raw, exists := opts["resilience"]; exists {
			resilience, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s resilience 必须是对象", name)
			}
			if err := weightedKeys(resilience, "enabled", "control"); err != nil {
				return err
			}
			enabled, ok := resilience["enabled"].(bool)
			if !ok {
				return fmt.Errorf("%s resilience.enabled 必须显式指定布尔值", name)
			}
			if !enabled {
				if len(resilience) != 1 {
					return fmt.Errorf("%s 禁用 resilience 时不能附带其他设置", name)
				}
				continue
			}
			control, ok := resilience["control"].(map[string]any)
			if !ok {
				return fmt.Errorf("%s 缺少 control 对象", name)
			}
			if err := weightedKeys(control, "base-url", "uuid", "timeout", "min-interval", "grace-interval"); err != nil {
				return err
			}
			address, _ := control["base-url"].(string)
			if err := validateControlURL(address); err != nil {
				return err
			}
			credential, _ := control["uuid"].(string)
			if !validControlUUID(credential) {
				return fmt.Errorf("%s control.uuid 必须是有效 UUID", name)
			}
			for _, key := range []string{"timeout", "min-interval", "grace-interval"} {
				if value, exists := control[key]; exists {
					if _, err := weightedInteger(value, 1, math.MaxInt64/1000000000); err != nil {
						return fmt.Errorf("%s %s：%w", name, key, err)
					}
				}
			}

		}
	}
	return nil
}

func validateWeightedNodes(value any, members map[string]bool, dynamic bool) error {
	nodes, ok := value.([]any)
	if !ok || len(nodes) == 0 || len(nodes) > 32 {
		return fmt.Errorf("nodes 必须包含 1 至 32 个服务器")
	}
	ids, owners := map[string]bool{}, map[string]bool{}
	var total int64
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("nodes 条目必须是对象")
		}
		if err := weightedKeys(node, "name", "id", "weight", "proxies"); err != nil {
			return err
		}
		name, _ := node["name"].(string)
		id, _ := node["id"].(string)
		if strings.TrimSpace(name) == "" || id == "" || strings.ContainsAny(id, "/ \t\r\n") || ids[id] {
			return fmt.Errorf("服务器名称不能为空，id 必须有效且唯一")
		}
		ids[id] = true
		weight, err := weightedInteger(node["weight"], 1, math.MaxInt64)
		if err != nil {
			return fmt.Errorf("服务器 %s 权重必须为正整数：%w", name, err)
		}
		if total > math.MaxInt64-weight {
			return fmt.Errorf("服务器权重总和溢出")
		}
		total += weight
		proxies, ok := node["proxies"].([]any)
		if !ok || len(proxies) == 0 {
			return fmt.Errorf("服务器 %s 必须包含代理", name)
		}
		for _, raw := range proxies {
			proxy, ok := raw.(string)
			if !ok || strings.TrimSpace(proxy) == "" || owners[proxy] || (!dynamic && !members[proxy]) {
				return fmt.Errorf("服务器 %s 代理归属重复或引用未知代理", name)
			}
			owners[proxy] = true
		}
	}
	for proxy := range members {
		if !owners[proxy] {
			return fmt.Errorf("nodes 缺少代理 %s", proxy)
		}
	}
	return nil
}

// Site identity uses the same IDNA normalization and per-kind ownership as Mihomo.
func validateSiteGroups(value any) error {
	groups, ok := value.([]any)
	if !ok {
		return fmt.Errorf("site-groups 必须是列表")
	}
	names := map[string]bool{}
	owners := map[string]map[string]string{"domains": {}, "domain-suffixes": {}}
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("site-groups 条目必须是对象")
		}
		if err := weightedKeys(group, "name", "domains", "domain-suffixes"); err != nil {
			return err
		}
		name, _ := group["name"].(string)
		if strings.TrimSpace(name) == "" || names[name] {
			return fmt.Errorf("站点组名称不能为空或重复：%s", name)
		}
		names[name] = true
		count := 0
		for _, key := range []string{"domains", "domain-suffixes"} {
			if group[key] == nil {
				continue
			}
			domains, ok := group[key].([]any)
			if !ok {
				return fmt.Errorf("%s 的 %s 必须是域名列表", name, key)
			}
			for _, value := range domains {
				raw, ok := value.(string)
				if !ok {
					return fmt.Errorf("%s 的域名必须是字符串", name)
				}
				domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
				if domain == "" || strings.ContainsAny(domain, " \t\r\n/\\@") {
					return fmt.Errorf("无效域名：%s", raw)
				}
				ascii, err := idna.Lookup.ToASCII(domain)
				ascii = strings.TrimSuffix(ascii, ".")
				if err != nil || ascii == "" {
					return fmt.Errorf("无效域名：%s", raw)
				}
				if owner, exists := owners[key][ascii]; exists && owner != name {
					return fmt.Errorf("%s 同时属于站点组 %s 和 %s", raw, owner, name)
				}
				owners[key][ascii] = name
				count++
			}
		}
		if count == 0 {
			return fmt.Errorf("站点组 %s 至少需要一个 domain 或 domain-suffix", name)
		}
	}
	return nil
}
