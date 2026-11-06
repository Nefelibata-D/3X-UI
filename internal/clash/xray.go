package clash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type xrayPolicy struct {
	key  string
	port int
	tags []string
}

// InjectXrayConfig routes every managed inbound through its policy's local
// gateway before the target dial. It never changes the stored template; the
// generated config is rebuilt on every Xray start/apply.
func InjectXrayConfig(cfg *xray.Config) error {
	if cfg == nil {
		return nil
	}
	rt := GetRuntime()
	if rt == nil {
		return nil
	}
	if rt.startErr != nil {
		return rt.startErr
	}
	rt.mu.RLock()
	policies := make([]*PolicyRuntime, 0, len(rt.policies))
	for _, pr := range rt.policies {
		policies = append(policies, pr)
	}
	rt.mu.RUnlock()
	if len(policies) == 0 {
		return nil
	}

	entries := make([]xrayPolicy, 0, len(policies))
	for _, pr := range policies {
		pr.mu.Lock()
		removing := pr.removing
		port := pr.port
		listening := pr.server != nil
		tags := make([]string, 0, len(pr.managed))
		for _, m := range pr.managed {
			if m.Tag != "" {
				tags = append(tags, m.Tag)
			}
		}
		pr.mu.Unlock()
		if removing || len(tags) == 0 {
			continue
		}
		if !listening || port <= 0 {
			return fmt.Errorf("clash: gateway unavailable for policy %s", pr.key)
		}
		entries = append(entries, xrayPolicy{key: pr.key, port: port, tags: tags})
	}
	if len(entries) == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	liveTags := make(map[string]struct{}, len(cfg.InboundConfigs))
	for i := range cfg.InboundConfigs {
		liveTags[cfg.InboundConfigs[i].Tag] = struct{}{}
	}

	var outbounds []map[string]any
	if len(cfg.OutboundConfigs) > 0 {
		if err := json.Unmarshal(cfg.OutboundConfigs, &outbounds); err != nil {
			return fmt.Errorf("clash: xray outbounds are not an array: %w", err)
		}
	}
	if len(outbounds) == 0 {
		return fmt.Errorf("clash: xray config has no outbounds")
	}
	existingOutbound := make(map[string]struct{}, len(outbounds))
	for _, ob := range outbounds {
		if tag, _ := ob["tag"].(string); tag != "" {
			existingOutbound[tag] = struct{}{}
		}
	}

	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			return fmt.Errorf("clash: xray routing is not an object: %w", err)
		}
	}
	if routing == nil {
		return fmt.Errorf("clash: routing must be an object")
	}
	rules, ok := routing["rules"].([]any)
	if routing["rules"] != nil && !ok {
		return fmt.Errorf("clash: routing rules must be an array")
	}
	newRules := make([]any, 0, len(entries))
	claimed := make(map[string]string, len(liveTags))
	for _, entry := range entries {
		filtered := make([]string, 0, len(entry.tags))
		for _, tag := range entry.tags {
			if _, ok := liveTags[tag]; !ok {
				continue
			}
			if owner, taken := claimed[tag]; taken && owner != entry.key {
				return fmt.Errorf("clash: inbound %q has conflicting policies %q and %q", tag, owner, entry.key)
			}
			claimed[tag] = entry.key
			filtered = append(filtered, tag)
		}
		if len(filtered) == 0 {
			continue
		}
		outboundTag := gatewayOutboundTag(entry.key, entry.port)
		if _, exists := existingOutbound[outboundTag]; exists {
			return fmt.Errorf("clash: outbound tag %q conflicts with policy %q", outboundTag, entry.key)
		}
		outbounds = append(outbounds, map[string]any{
			"tag":      outboundTag,
			"protocol": "socks",
			"settings": map[string]any{
				"servers": []any{
					map[string]any{
						"address": "127.0.0.1",
						"port":    entry.port,
					},
				},
			},
		})
		existingOutbound[outboundTag] = struct{}{}
		newRules = append(newRules, map[string]any{
			"type":        "field",
			"inboundTag":  filtered,
			"outboundTag": outboundTag,
		})
	}
	if len(newRules) == 0 {
		return nil
	}
	combined := make([]any, 0, len(newRules)+len(rules))
	combined = append(combined, newRules...)
	combined = append(combined, rules...)
	routing["rules"] = combined

	outJSON, err := json.Marshal(outbounds)
	if err != nil {
		return fmt.Errorf("clash: marshal xray outbounds: %w", err)
	}
	routeJSON, err := json.Marshal(routing)
	if err != nil {
		return fmt.Errorf("clash: marshal xray routing: %w", err)
	}
	cfg.OutboundConfigs = json_util.RawMessage(outJSON)
	cfg.RouterConfig = json_util.RawMessage(routeJSON)
	return nil
}

func gatewayOutboundTag(key string, port int) string {
	sum := sha256.Sum256([]byte(key + "|" + strconv.Itoa(port)))
	return "clash-gw-" + hex.EncodeToString(sum[:6])
}

func (rt *Runtime) tagConflict(masterGuid string, clientID int, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	owned := make(map[string]string, len(tags))
	for _, tag := range tags {
		owned[tag] = policyKey(masterGuid, clientID)
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for key, pr := range rt.policies {
		if key == policyKey(masterGuid, clientID) {
			continue
		}
		pr.mu.Lock()
		removing := pr.removing
		managed := append([]ManagedInbound(nil), pr.managed...)
		pr.mu.Unlock()
		if removing {
			continue
		}
		for _, m := range managed {
			if owner, ok := owned[m.Tag]; ok && owner != key {
				return &ConflictError{Tag: m.Tag, Owner: key}
			}
		}
	}
	return nil
}

type ConflictError struct {
	Tag   string
	Owner string
}

func (e *ConflictError) Error() string {
	return "inbound " + e.Tag + " is already managed by " + e.Owner
}
