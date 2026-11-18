package subscription

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

// Place runs missing from primary beside their nearest surviving reference in reference.
func anchoredOrder(primary, reference []string) []string {
	present := map[string]bool{}
	for _, id := range primary {
		present[id] = true
	}
	after := map[string][]string{}
	var prefix, pending []string
	var left, first string
	for _, id := range reference {
		if !present[id] {
			pending = append(pending, id)
			continue
		}
		if first == "" {
			first = id
		}
		if left != "" {
			after[left] = append(after[left], pending...)
		} else if len(pending) > 0 {
			// Put a leading run immediately before its right anchor, even if that anchor moved.
			prefix = pending
		}
		pending = nil
		left = id
	}
	if left == "" {
		return append(slices.Clone(primary), pending...)
	}
	after[left] = append(after[left], pending...)
	var result []string
	for _, id := range primary {
		if id == first {
			result = append(result, prefix...)
		}
		result = append(result, id)
		result = append(result, after[id]...)
	}
	return result
}

func projectOrder(list []string, keep func(string) bool) []string {
	var out []string
	for _, id := range list {
		if keep(id) {
			out = append(out, id)
		}
	}
	return out
}

// Old shared anchors identify which side reordered them. A new template reorder
// wins when both sides reorder the same anchors; private runs retain their neighbors.
func rebaseOrder(old, fresh, local []string) []string {
	sets := func(list []string) map[string]bool {
		m := map[string]bool{}
		for _, id := range list {
			m[id] = true
		}
		return m
	}
	o, n, l := sets(old), sets(fresh), sets(local)
	shared := func(id string) bool { return o[id] && n[id] && l[id] }
	fresh = projectOrder(fresh, func(id string) bool { return !o[id] || l[id] })
	local = projectOrder(local, func(id string) bool { return !o[id] || n[id] })
	if slices.Equal(projectOrder(old, shared), projectOrder(fresh, shared)) {
		return anchoredOrder(local, fresh)
	}
	return anchoredOrder(fresh, local)
}

func sectionIDs(config map[string]any, key string) ([]string, error) {
	var ids []string
	list, _ := config[key].([]any)
	for _, v := range list {
		var id string
		if key == "rules" {
			s, _ := v.(string)
			var err error
			id, err = RuleIdentity(s)
			if err != nil {
				return nil, err
			}
		} else {
			m, _ := v.(map[string]any)
			id, _ = m["name"].(string)
			if id == "" {
				return nil, fmt.Errorf("missing group name")
			}
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Occurrence numbers distinguish repeated predicates without collapsing rules.
func mergeRows(old, fresh, local []any, identity func(any) (string, error), combine func(any, any, any) (any, error)) ([]any, error) {
	index := func(rows []any) ([]string, map[string]any, error) {
		counts := map[string]int{}
		values := map[string]any{}
		var ids []string
		for _, v := range rows {
			k, err := identity(v)
			if err != nil {
				return nil, nil, err
			}
			counts[k]++
			id := fmt.Sprintf("%d:%s", counts[k], k)
			ids = append(ids, id)
			values[id] = v
		}
		return ids, values, nil
	}
	a, av, e := index(old)
	if e != nil {
		return nil, e
	}
	b, bv, e := index(fresh)
	if e != nil {
		return nil, e
	}
	c, cv, e := index(local)
	if e != nil {
		return nil, e
	}
	out := []any{}
	for _, id := range rebaseOrder(a, b, c) {
		v := bv[id]
		if custom, ok := cv[id]; ok && !reflect.DeepEqual(custom, av[id]) {
			v = custom
			if combine != nil && av[id] != nil && bv[id] != nil {
				v, e = combine(av[id], bv[id], custom)
				if e != nil {
					return nil, e
				}
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func rebaseBinding(old, fresh map[string]any, b *SubscriptionBinding) error {
	var overlay map[string]any
	if err := yaml.Unmarshal([]byte(b.Content), &overlay); err != nil {
		return err
	}
	for _, section := range []struct {
		key   string
		order *[]string
	}{{"rules", &b.RuleOrder}, {"proxy-groups", &b.GroupOrder}} {
		if len(*section.order) == 0 {
			continue
		}
		a, e := sectionIDs(old, section.key)
		if e != nil {
			return e
		}
		n, e := sectionIDs(fresh, section.key)
		if e != nil {
			return e
		}
		// A subscription replacement survives removal of the corresponding template entry.
		own, e := sectionIDs(overlay, section.key)
		if e != nil {
			return e
		}
		a = projectOrder(a, func(id string) bool { return !slices.Contains(own, id) || slices.Contains(n, id) })
		*section.order = rebaseOrder(a, n, *section.order)
	}
	changed := false
	for _, key := range []string{"site-blacklist", "site-sticky"} {
		local, ok := overlay[key].([]any)
		if !ok || reflect.DeepEqual(old[key], fresh[key]) {
			continue
		}
		a, _ := old[key].([]any)
		n, _ := fresh[key].([]any)
		group := func(v any) (string, error) {
			m, _ := v.(map[string]any)
			s, _ := m["group"].(string)
			if s == "" {
				return "", fmt.Errorf("missing group in %s", key)
			}
			return s, nil
		}
		rows, e := mergeRows(a, n, local, group, func(a, b, c any) (any, error) {
			rules := func(v any) []any { r, _ := v.(map[string]any)["rules"].([]any); return r }
			id := func(v any) (string, error) {
				s, ok := v.(string)
				if !ok {
					return "", fmt.Errorf("invalid %s rule", key)
				}
				if key == "site-blacklist" {
					k, _, e := ParseBlacklistRule(s)
					return k, e
				}
				last := strings.LastIndexByte(s, ',')
				if last < 0 {
					return "", fmt.Errorf("invalid sticky rule")
				}
				prev := strings.LastIndexByte(s[:last], ',')
				if prev < 0 {
					return "", fmt.Errorf("invalid sticky rule")
				}
				return s[:prev], nil
			}
			r, e := mergeRows(rules(a), rules(b), rules(c), id, nil)
			return map[string]any{"group": c.(map[string]any)["group"], "rules": r}, e
		})
		if e != nil {
			return e
		}
		if reflect.DeepEqual(rows, n) {
			delete(overlay, key)
			changed = true
		} else if !reflect.DeepEqual(local, rows) {
			overlay[key] = rows
			changed = true
		}
	}
	if changed {
		raw, e := yaml.Marshal(overlay)
		if e != nil {
			return e
		}
		b.Content = string(raw)
	}
	return nil
}
