package subscription

import (
	"fmt"
)

func ValidatePublishedSubscription(config map[string]any, proxies []map[string]any, dialers map[string]string) error {
	if err := ValidateWeightedSubscription(config); err != nil {
		return err
	}
	groups, err := subscriptionGroups(config)
	if err != nil {
		return err
	}
	actual := map[string]map[string]any{}
	graph := map[string][]string{}
	for _, p := range proxies {
		name, _ := p["name"].(string)
		kind, _ := p["type"].(string)
		if name == "" || actual[name] != nil {
			return fmt.Errorf("代理名称为空或重复：%s", name)
		}
		if kind != "direct" && kind != "reject" && kind != "dns" {
			actual[name] = p
		}
	}
	for name, via := range dialers {
		if actual[name] == nil {
			return fmt.Errorf("链式代理节点不存在或未获授权：%s", name)
		}
		if via == "" {
			delete(actual[name], "dialer-proxy")
		} else {
			actual[name]["dialer-proxy"] = via
		}
	}
	for name, p := range actual {
		if via, ok := p["dialer-proxy"].(string); ok && via != "" {
			if via != "DIRECT" && actual[via] == nil && groups[via] == nil {
				return fmt.Errorf("链式代理引用不存在：%s", via)
			}
			graph[name] = []string{via}
		}
	}
	candidates := map[string]map[string]bool{}
	for name, g := range groups {
		members, _ := g["proxies"].([]any)
		set := map[string]bool{}
		for _, member := range members {
			n, _ := member.(string)
			graph[name] = append(graph[name], n)
			if actual[n] == nil {
				continue
			}
			set[n] = true
		}
		candidates[name] = set
		minimum := 2
		if g["strategy"] == "weighted-sticky" {
			minimum = 1
		}
		if g["type"] == "load-balance" && len(set) < minimum {
			return fmt.Errorf("节点组 %s 发布前必须配置至少 %d 个有效代理节点", name, minimum)
		}
	}
	active, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if active[name] {
			return fmt.Errorf("链式代理或节点组循环引用：%s", name)
		}
		if done[name] {
			return nil
		}
		active[name] = true
		for _, next := range graph[name] {
			if err := visit(next); err != nil {
				return err
			}
		}
		delete(active, name)
		done[name] = true
		return nil
	}
	for name := range graph {
		if err := visit(name); err != nil {
			return err
		}
	}
	blacklists, err := validateSiteBlacklist(config)
	if err != nil {
		return err
	}
	for _, entry := range blacklists {
		if len(entry.proxies) == 0 {
			return fmt.Errorf("发布前请配置黑名单节点：%s", entry.group)
		}
		for _, proxy := range entry.proxies {
			if actual[proxy] == nil || !candidates[entry.group][proxy] {
				return fmt.Errorf("黑名单节点 %s 不在节点组 %s 的有效成员中", proxy, entry.group)
			}
		}
	}
	return nil
}
