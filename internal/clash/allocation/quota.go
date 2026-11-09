package allocation

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
	"gorm.io/gorm"
)

var quotaMu sync.Mutex

type NodeWeightInput struct {
	CapacityBytes *int64                 `json:"capacityBytes"`
	ResetSchedule *gateway.ResetSchedule `json:"resetSchedule"`
	DefaultWeight *int64                 `json:"defaultWeight"`
	NodeKey       string                 `json:"nodeKey"`
}

func SaveNodeWeights(cat *catalog.Catalog, inputs []NodeWeightInput) error {
	quotaMu.Lock()
	defer quotaMu.Unlock()
	known := make(map[string]catalog.NodeInfo, len(cat.Nodes))
	for _, n := range cat.Nodes {
		known[n.Key] = n
	}
	seen := make(map[string]struct{}, len(inputs))
	rows := make([]NodeWeight, 0, len(inputs))
	existing, err := ListNodeWeights()
	if err != nil {
		return err
	}
	configs := NodeConfigMap(cat, existing)
	for _, in := range inputs {
		node, ok := known[in.NodeKey]
		if !ok {
			return fmt.Errorf("unknown outlet %q", in.NodeKey)
		}
		if _, dup := seen[in.NodeKey]; dup {
			return fmt.Errorf("duplicate outlet %q", in.NodeKey)
		}
		seen[in.NodeKey] = struct{}{}
		weight := configs[in.NodeKey].DefaultWeight
		if in.DefaultWeight != nil {
			weight = *in.DefaultWeight
		}
		if weight <= 0 {
			return ErrNonPositiveWeight
		}
		var nodeID *int
		if !node.IsLocal {
			id := node.ID
			nodeID = &id
		}
		cfg := configs[in.NodeKey]
		if in.CapacityBytes != nil {
			cfg.CapacityBytes = *in.CapacityBytes
		}
		if in.ResetSchedule != nil {
			cfg.ResetSchedule = *in.ResetSchedule
		}
		if cfg.CapacityBytes < 0 {
			return errors.New("节点总额度不能为负数")
		}
		if cfg.ResetSchedule.Disabled {
			return errors.New("节点须设置每月重置周期")
		}
		if err := cfg.ResetSchedule.Validate(); err != nil {
			return err
		}
		configs[in.NodeKey] = cfg
		rows = append(rows, NodeWeight{
			CapacityBytes: cfg.CapacityBytes, ResetSchedule: cfg.ResetSchedule,
			NodeKey:       in.NodeKey,
			NodeID:        nodeID,
			NodeGuid:      node.Guid,
			DefaultWeight: weight,
		})
	}
	ratios, err := ListRatios()
	if err != nil {
		return err
	}
	if err := ValidateNodeAllocations(cat, configs, RatioMap(ratios), -1); err != nil {
		return err
	}
	return UpsertNodeWeights(rows)
}

func SaveClientRatio(cat *catalog.Catalog, clientID int, mode string, weights, limits map[string]int64, resetModes ...string) error {
	quotaMu.Lock()
	defer quotaMu.Unlock()
	if err := ValidateMode(mode); err != nil {
		return err
	}
	client, ok := cat.ClientByID(clientID)
	if !ok {
		return catalog.ErrClientNotFound
	}
	if err := ValidateWeights(weights); err != nil {
		return err
	}
	known := make(map[string]struct{}, len(cat.Nodes))
	for _, n := range cat.Nodes {
		known[n.Key] = struct{}{}
	}
	for key := range weights {
		if _, ok := known[key]; !ok {
			return fmt.Errorf("unknown outlet %q", key)
		}
	}
	for key, limit := range limits {
		if _, ok := known[key]; !ok {
			return fmt.Errorf("unknown outlet %q", key)
		}
		if limit < 0 {
			return errors.New("client node limit must not be negative")
		}
	}
	if client.TotalBytes > 0 {
		keys := cat.AuthorizedNodeKeys(clientID)
		for _, key := range keys {
			if limits[key] == 0 {
				return errors.New("客户端总流量有限时，每个节点额度也必须有限")
			}
		}
		var allocated int64
		for _, key := range keys {
			limit := limits[key]
			if limit > client.TotalBytes-allocated {
				return errors.New("节点流量额度合计不能超过客户端总流量上限")
			}
			allocated += limit
		}
	}

	if mode == ModeCustom {
		for _, key := range cat.AuthorizedNodeKeys(clientID) {
			if weights[key] <= 0 {
				return fmt.Errorf("节点 %s：%w", key, ErrNonPositiveWeight)
			}
		}
		nodeWeights, err := ListNodeWeights()
		if err != nil {
			return err
		}
		ratio := &ClientRatio{ClientID: clientID, Mode: mode}
		raw, err := EncodeWeights(weights)
		if err != nil {
			return err
		}
		ratio.WeightsRaw = raw
		if _, err := EffectiveWeights(cat, clientID, NodeConfigMap(cat, nodeWeights), ratio); err != nil {
			return err
		}
	}
	raw, err := EncodeWeights(weights)
	if err != nil {
		return err
	}
	limitsRaw, err := EncodeWeights(limits)
	if err != nil {
		return err
	}
	resetMode := "node"
	if old, err := GetRatio(clientID); err == nil && old.ResetMode != "" {
		resetMode = old.ResetMode
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if len(resetModes) > 0 && resetModes[0] != "" {
		resetMode = resetModes[0]
	}
	if resetMode != "node" && resetMode != "manual" {
		return errors.New("重置模式须为 node 或 manual")
	}
	row := ClientRatio{
		ResetMode:  resetMode,
		ClientID:   client.ID,
		Email:      client.Email,
		Mode:       mode,
		WeightsRaw: raw,
		LimitsRaw:  limitsRaw,
	}
	rows, err := ListRatios()
	if err != nil {
		return err
	}
	configs, err := ListNodeWeights()
	if err != nil {
		return err
	}
	ratios := RatioMap(rows)
	ratios[clientID] = row
	if err := ValidateNodeAllocations(cat, NodeConfigMap(cat, configs), ratios, clientID); err != nil {
		return err
	}
	return UpsertRatio(row)
}

func ValidateClientTotalLimit(clientID int, totalBytes int64) error {
	if totalBytes <= 0 {
		return nil
	}
	ratio, err := GetRatio(clientID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	limits := ratio.Limits()
	if len(limits) == 0 {
		return nil
	}
	var allocated int64
	for _, limit := range limits {
		if limit <= 0 {
			return errors.New("客户端总流量有限时，每个节点额度也必须有限")
		}
		if limit > totalBytes-allocated {
			return errors.New("节点流量额度合计不能超过客户端总流量上限")
		}
		allocated += limit
	}
	return nil
}
