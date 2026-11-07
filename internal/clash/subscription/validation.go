package subscription

import (
	"fmt"
	"strconv"
	"strings"
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
		opts, _ := g["weighted-sticky"].(map[string]any)
		weights, _ := opts["weights"].(map[string]any)
		for _, member := range members {
			n, _ := member.(string)
			graph[name] = append(graph[name], n)
			if actual[n] == nil {
				continue
			}
			weight := any(1)
			if v, ok := opts["default-weight"]; ok {
				weight = v
			}
			if v, ok := weights[n]; ok {
				weight = v
			}
			number, err := strconv.ParseFloat(fmt.Sprint(weight), 64)
			if err != nil || number < 0 {
				return fmt.Errorf("节点组 %s 的节点权重无效", name)
			}
			if number > 0 {
				set[n] = true
			}
		}
		candidates[name] = set
		if g["type"] == "load-balance" && len(set) < 2 {
			return fmt.Errorf("节点组 %s 发布前必须配置至少两个有效代理节点", name)
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
	excluded := map[string]map[string]bool{}
	list, _ := config["site-blacklist"].([]any)
	for _, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("黑名单配置必须是对象")
		}
		proxy, _ := entry["proxy"].(string)
		if strings.TrimSpace(proxy) == "" || actual[proxy] == nil {
			return fmt.Errorf("发布前请配置有效的黑名单节点：%s", proxy)
		}
		scope, _ := entry["group"].(string)
		targets := []string{}
		if scope != "" {
			targets = append(targets, scope)
		} else {
			for name, g := range groups {
				if g["strategy"] == "weighted-sticky" {
					targets = append(targets, name)
				}
			}
		}
		for _, name := range targets {
			g := groups[name]
			if g == nil || g["type"] != "load-balance" || g["strategy"] != "weighted-sticky" {
				return fmt.Errorf("黑名单所属负载均衡节点组不存在：%s", name)
			}
			if !candidates[name][proxy] {
				return fmt.Errorf("黑名单节点 %s 不在节点组 %s 的有效成员中", proxy, name)
			}
			if excluded[name] == nil {
				excluded[name] = map[string]bool{}
			}
			excluded[name][proxy] = true
		}
	}
	for name, nodes := range excluded {
		if len(nodes) >= len(candidates[name]) {
			return fmt.Errorf("节点组 %s 必须保留至少一个未配置黑名单的有效代理", name)
		}
	}
	return nil
}
