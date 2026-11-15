package subscription

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// site-sticky rules use the final two fields for the binding lifetime and renewal.
func validateStickyRule(line string, providers map[string]any) error {
	end := strings.LastIndexByte(line, ',')
	if end < 0 {
		return fmt.Errorf("缺少续期标志")
	}
	renew := strings.TrimSpace(line[end+1:])
	if renew != "true" && renew != "false" {
		return fmt.Errorf("续期标志必须是 true 或 false")
	}
	head := line[:end]
	end = strings.LastIndexByte(head, ',')
	if end < 0 {
		return fmt.Errorf("缺少有效秒数")
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(head[end+1:]))
	if err != nil || seconds <= 0 || int64(seconds) > math.MaxInt64/int64(time.Second) {
		return fmt.Errorf("有效秒数必须是 1 到 9223372036 的整数")
	}
	rule := strings.TrimSpace(head[:end])
	kind, payload, ok := strings.Cut(rule, ",")
	if !ok {
		return fmt.Errorf("缺少匹配内容")
	}
	if strings.EqualFold(strings.TrimSpace(kind), "RULE-SET") {
		name := strings.TrimSpace(payload)
		provider, _ := providers[name].(map[string]any)
		if name == "" || strings.Contains(name, ",") || provider == nil || provider["behavior"] != "classical" || (provider["format"] != nil && provider["format"] != "" && provider["format"] != "yaml" && provider["format"] != "text") {
			return fmt.Errorf("规则集 %q 必须存在且为 classical YAML/text", name)
		}
		return nil
	}
	return validateRules([]string{rule}, true)
}

func validateSiteSticky(config map[string]any, groups map[string]map[string]any) error {
	raw := config["site-sticky"]
	if raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("site-sticky 必须是列表")
	}
	seen := map[string]bool{}
	providers, _ := config["rule-providers"].(map[string]any)
	for i, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("site-sticky[%d] 必须是对象", i)
		}
		if err := weightedKeys(entry, "group", "rules"); err != nil {
			return fmt.Errorf("site-sticky[%d]: %w", i, err)
		}
		name, _ := entry["group"].(string)
		g := groups[name]
		if strings.TrimSpace(name) == "" || seen[name] || g["type"] != "load-balance" || g["strategy"] != "weighted-sticky" {
			return fmt.Errorf("site-sticky[%d] 必须引用有效且未重复的 weighted-sticky 组：%q", i, name)
		}
		seen[name] = true
		rules, ok := entry["rules"].([]any)
		if !ok || len(rules) == 0 {
			return fmt.Errorf("site-sticky[%d].rules 必须是非空列表", i)
		}
		for j, r := range rules {
			line, ok := r.(string)
			if !ok {
				return fmt.Errorf("site-sticky[%d].rules[%d] 必须是字符串", i, j)
			}
			if err := validateStickyRule(line, providers); err != nil {
				return fmt.Errorf("site-sticky[%d].rules[%d]: %w", i, j, err)
			}
		}
	}
	return nil
}
