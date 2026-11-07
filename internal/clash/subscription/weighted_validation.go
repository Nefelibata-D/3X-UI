package subscription

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strings"

	"github.com/google/uuid"
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
		if err := weightedKeys(opts, "idle-timeout", "default-weight", "weights", "max-bindings", "site-groups", "resilience"); err != nil {
			return err
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
		fallback := int64(1)
		if value, exists := opts["default-weight"]; exists {
			fallback, err = weightedInteger(value, 0, math.MaxInt64)
			if err != nil {
				return fmt.Errorf("%s default-weight：%w", name, err)
			}
		}
		members, _ := group["proxies"].([]any)
		known := map[string]bool{}
		for _, value := range members {
			if member, ok := value.(string); ok {
				if groups[member] != nil {
					return fmt.Errorf("加权组 %s 不支持嵌套代理组 %s", name, member)
				}
				known[member] = true
			}
		}
		use, _ := group["use"].([]any)
		dynamic := len(use) > 0 || group["include-all"] == true || group["include-all-proxies"] == true || group["include-all-providers"] == true
		weights := map[string]int64{}
		if raw, exists := opts["weights"]; exists {
			mapping, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("%s weights 必须是对象", name)
			}
			for member, value := range mapping {
				if strings.TrimSpace(member) == "" || (!dynamic && !known[member]) {
					return fmt.Errorf("%s weights 引用未知代理 %s", name, member)
				}
				weight, err := weightedInteger(value, 0, math.MaxInt64)
				if err != nil {
					return fmt.Errorf("%s weights[%s]：%w", name, member, err)
				}
				weights[member] = weight
			}
		}
		var total int64
		for member := range known {
			weight, ok := weights[member]
			if !ok {
				weight = fallback
			}
			if total > math.MaxInt64-weight {
				return fmt.Errorf("%s 权重总和溢出", name)
			}
			total += weight
		}
		if !dynamic && len(known) > 0 && total == 0 {
			return fmt.Errorf("%s 所有代理权重为零", name)
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
			if err := weightedKeys(control, "base-url", "uuid", "timeout", "min-interval", "grace-interval", "node-ids"); err != nil {
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
			ids, ok := control["node-ids"].(map[string]any)
			if !ok || len(ids) == 0 {
				return fmt.Errorf("%s node-ids 必须是非空对象", name)
			}
			distinct := map[string]bool{}
			for member, value := range ids {
				id, ok := value.(string)
				if !ok || strings.TrimSpace(id) == "" || strings.ContainsAny(id, "/ \t\r\n") || (!dynamic && !known[member]) {
					return fmt.Errorf("%s node-ids[%s] 无效或不是组成员", name, member)
				}
				distinct[id] = true
			}
			if len(distinct) > 32 {
				return fmt.Errorf("%s node-ids 最多支持 32 个服务器", name)
			}
			for member := range known {
				if _, exists := ids[member]; !exists {
					return fmt.Errorf("%s node-ids 缺少代理 %s", name, member)
				}
			}
		}
	}
	return nil
}
