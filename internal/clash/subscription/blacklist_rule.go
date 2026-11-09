package subscription

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const blacklistNameToken = `\s*(?:"(?:\\.|[^"\\])*"|[^,"\[\]\r\n]+)\s*`

var blacklistLinePattern = regexp.MustCompile(`^(.*),\s*\[((?:` + blacklistNameToken + `(?:,` + blacklistNameToken + `)*)?)\]\s*$`)
var blacklistNamesPattern = regexp.MustCompile(blacklistNameToken)

func ParseBlacklistRule(line string) (string, []string, error) {
	parts := blacklistLinePattern.FindStringSubmatch(line)
	if parts == nil || strings.ContainsAny(line, "\r\n") {
		return "", nil, fmt.Errorf("blacklist rule must be TYPE,payload,[proxy names]: %q", line)
	}
	names := []string{}
	seen := map[string]bool{}
	for _, token := range blacklistNamesPattern.FindAllString(parts[2], -1) {
		name := strings.TrimSpace(token)
		if strings.HasPrefix(name, `"`) {
			if err := json.Unmarshal([]byte(name), &name); err != nil {
				return "", nil, fmt.Errorf("invalid blacklist proxy name: %w", err)
			}
		}
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") || seen[name] {
			return "", nil, fmt.Errorf("empty or duplicate blacklist proxy name: %q", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	return strings.TrimSpace(parts[1]), names, nil
}

type blacklistRule struct {
	group   string
	proxies []string
}

func validateSiteBlacklist(config map[string]any) ([]blacklistRule, error) {
	raw, exists := config["site-blacklist"]
	if !exists {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("site-blacklist must be a list")
	}
	groups, err := subscriptionGroups(config)
	if err != nil {
		return nil, err
	}
	providers, _ := config["rule-providers"].(map[string]any)
	seen := map[string]bool{}
	result := []blacklistRule{}
	for _, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("黑名单配置必须是对象")
		}
		if err := weightedKeys(entry, "group", "rules"); err != nil {
			return nil, err
		}
		group, ok := entry["group"].(string)
		if !ok || strings.TrimSpace(group) == "" || seen[group] {
			return nil, fmt.Errorf("黑名单 group 必须非空且不能重复")
		}
		seen[group] = true
		g := groups[group]
		if g == nil || g["type"] != "load-balance" || g["strategy"] != "weighted-sticky" {
			return nil, fmt.Errorf("黑名单所属负载均衡节点组不存在：%s", group)
		}
		lines, ok := entry["rules"].([]any)
		if !ok || len(lines) == 0 {
			return nil, fmt.Errorf("黑名单 rules 必须是非空列表")
		}
		for _, raw := range lines {
			line, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("黑名单规则必须是字符串")
			}
			rule, proxies, err := ParseBlacklistRule(line)
			if err != nil {
				return nil, err
			}
			kind, payload, found := strings.Cut(rule, ",")
			if !found || strings.TrimSpace(payload) == "" {
				return nil, fmt.Errorf("无效黑名单规则：%s", rule)
			}
			if kind == "RULE-SET" {
				provider, _ := providers[payload].(map[string]any)
				if provider == nil || provider["behavior"] != "classical" || provider["format"] == "mrs" {
					return nil, fmt.Errorf("黑名单规则集 %s 必须是 classical YAML/text", payload)
				}
			} else if err := validateRules([]string{rule}, true); err != nil {
				return nil, err
			}
			result = append(result, blacklistRule{group: group, proxies: proxies})
		}
	}
	return result, nil
}
