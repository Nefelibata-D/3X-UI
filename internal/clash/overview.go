package clash

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
)

type NodeOverview struct {
	CapacityBytes  int64                 `json:"capacityBytes"`
	AllocatedBytes int64                 `json:"allocatedBytes"`
	ResetSchedule  gateway.ResetSchedule `json:"resetSchedule"`
	DefaultWeight  int64                 `json:"defaultWeight"`
	Key            string                `json:"key"`
	ID             int                   `json:"id"`
	Name           string                `json:"name"`
	Guid           string                `json:"guid"`
	Enabled        bool                  `json:"enabled"`
	Status         string                `json:"status"`
	IsLocal        bool                  `json:"isLocal"`
	DefaultRatio   float64               `json:"defaultRatio"`
}

type PolicyOverview struct {
	MasterGuid        string           `json:"masterGuid"`
	NodeKey           string           `json:"nodeKey"`
	NodeName          string           `json:"nodeName"`
	ClientID          int              `json:"clientId"`
	ClientEmail       string           `json:"clientEmail"`
	Revision          int64            `json:"revision"`
	Status            string           `json:"status"`
	LastError         string           `json:"lastError"`
	LastReportAt      int64            `json:"lastReportAt"`
	ResetPending      bool             `json:"resetPending"`
	Remove            bool             `json:"remove"`
	State             string           `json:"state"`
	BudgetBytes       int64            `json:"budgetBytes"`
	EffectiveRatio    float64          `json:"effectiveRatio"`
	UsageUp           int64            `json:"usageUp"`
	UsageDown         int64            `json:"usageDown"`
	PeriodStart       int64            `json:"periodStart"`
	PeriodEnd         int64            `json:"periodEnd"`
	GraceStartedAt    int64            `json:"graceStartedAt"`
	GraceDeadline     int64            `json:"graceDeadline"`
	GraceExtraBytes   int64            `json:"graceExtraBytes"`
	GatewayPort       int              `json:"gatewayPort"`
	GatewayListening  bool             `json:"gatewayListening"`
	ActiveConnections int              `json:"activeConnections"`
	ManagedInbounds   []ManagedInbound `json:"managedInbounds"`
	Applied           bool             `json:"applied"`
	ApplyError        string           `json:"applyError"`
	DisabledAt        int64            `json:"disabledAt"`
	Remote            bool             `json:"remote"`
}

type Overview struct {
	PanelGuid   string           `json:"panelGuid"`
	Timezone    string           `json:"timezone"`
	GeneratedAt int64            `json:"generatedAt"`
	Nodes       []NodeOverview   `json:"nodes"`
	Policies    []PolicyOverview `json:"policies"`
	Sync        SyncStatus       `json:"sync"`
}

type ClientNodeView struct {
	CapacityBytes  int64                 `json:"capacityBytes"`
	AllocatedBytes int64                 `json:"allocatedBytes"`
	ResetSchedule  gateway.ResetSchedule `json:"resetSchedule"`
	PeriodEnd      int64                 `json:"periodEnd"`
	UsageUp        int64                 `json:"usageUp"`
	UsageDown      int64                 `json:"usageDown"`
	UsageKnown     bool                  `json:"usageKnown"`
	Applied        bool                  `json:"applied"`
	ApplyError     string                `json:"applyError"`
	State          string                `json:"state"`
	DefaultWeight  int64                 `json:"defaultWeight"`
	NodeKey        string                `json:"nodeKey"`
	Name           string                `json:"name"`
	Guid           string                `json:"guid"`
	Enabled        bool                  `json:"enabled"`
	BudgetBytes    int64                 `json:"budgetBytes"`
	Selectable     bool                  `json:"selectable"`
	DefaultRatio   float64               `json:"defaultRatio"`
	OverrideWeight int64                 `json:"overrideWeight"`
	EffectiveRatio float64               `json:"effectiveRatio"`
}

type ClientRatioView struct {
	Timezone    string           `json:"timezone"`
	ResetMode   string           `json:"resetMode"`
	TotalBytes  int64            `json:"totalBytes"`
	ClientID    int              `json:"clientId"`
	Email       string           `json:"email"`
	Mode        string           `json:"mode"`
	WeightError string           `json:"weightError,omitempty"`
	Nodes       []ClientNodeView `json:"nodes"`
}

func NodeDefaultRatios(cat *Catalog, nodes map[string]NodeWeight) map[string]float64 {
	out := make(map[string]float64, len(cat.Nodes))
	weights := make([]WeightInput, 0, len(cat.Nodes))
	for _, n := range cat.Nodes {
		if !n.Enabled {
			continue
		}
		weights = append(weights, WeightInput{Key: n.Key, Weight: nodes[n.Key].DefaultWeight})
	}
	ratios, err := NormalizeWeights(weights)
	if err != nil {
		return out
	}
	for _, r := range ratios {
		out[r.Key] = r.Ratio
	}
	return out
}

func BuildClientRatioView(cat *Catalog, clientID int, nodes map[string]NodeWeight, ratio *ClientRatio) (ClientRatioView, error) {
	client, ok := cat.ClientByID(clientID)
	if !ok {
		return ClientRatioView{}, ErrClientNotFound
	}
	view := ClientRatioView{
		Timezone:   cat.Timezone,
		ResetMode:  "node",
		ClientID:   client.ID,
		TotalBytes: client.TotalBytes,
		Email:      client.Email,
		Mode:       ModeInherit,
		Nodes:      []ClientNodeView{},
	}
	weights := map[string]int64{}
	limits := map[string]int64{}
	if ratio != nil {
		if ratio.ResetMode != "" {
			view.ResetMode = ratio.ResetMode
		}
		view.Mode = ratio.Mode
		if view.Mode == "" {
			view.Mode = ModeInherit
		}
		weights = ratio.Weights()
		limits = ratio.Limits()
	}
	authorized := cat.AuthorizedNodeKeys(clientID)
	defaultInputs := make([]WeightInput, 0, len(authorized))
	effectiveInputs := make([]WeightInput, 0, len(authorized))
	for _, key := range authorized {
		node, ok := cat.NodeByKey(key)
		if !ok {
			continue
		}
		budget := limits[key]
		entry := ClientNodeView{
			CapacityBytes:  nodes[key].CapacityBytes,
			ResetSchedule:  nodes[key].ResetSchedule,
			NodeKey:        key,
			Name:           node.Name,
			Guid:           node.Guid,
			Enabled:        node.Enabled,
			BudgetBytes:    budget,
			Selectable:     node.Enabled,
			DefaultWeight:  nodes[key].DefaultWeight,
			OverrideWeight: weights[key],
		}
		if entry.Selectable {
			defaultInputs = append(defaultInputs, WeightInput{Key: key, Weight: nodes[key].DefaultWeight})
			w := nodes[key].DefaultWeight
			if view.Mode == ModeCustom {
				w = weights[key]
			}
			effectiveInputs = append(effectiveInputs, WeightInput{Key: key, Weight: w})
		}
		view.Nodes = append(view.Nodes, entry)
	}
	if defaults, err := NormalizeWeights(defaultInputs); err == nil {
		applyRatios(view.Nodes, defaults, func(n *ClientNodeView, ratio float64) {
			n.DefaultRatio = ratio
		})
	}
	if effective, err := NormalizeWeights(effectiveInputs); err != nil {
		view.WeightError = err.Error()
	} else {
		applyRatios(view.Nodes, effective, func(n *ClientNodeView, ratio float64) {
			n.EffectiveRatio = ratio
		})
	}
	return view, nil
}

func applyRatios(nodes []ClientNodeView, ratios []RatioResult, set func(*ClientNodeView, float64)) {
	for i := range nodes {
		for _, r := range ratios {
			if r.Key == nodes[i].NodeKey {
				set(&nodes[i], r.Ratio)
			}
		}
	}
}

func BuildOverview(cat *Catalog) (Overview, error) {
	nodeWeights, err := ListNodeWeights()
	if err != nil {
		return Overview{}, err
	}
	deliveries, err := ListDeliveries()
	if err != nil {
		return Overview{}, err
	}
	applied, err := ListAppliedPolicies()
	if err != nil {
		return Overview{}, err
	}
	configs := NodeConfigMap(cat, nodeWeights)
	ratios, err := ListRatios()
	if err != nil {
		return Overview{}, err
	}
	allocated := NodeAllocations(cat, RatioMap(ratios))
	defaults := NodeDefaultRatios(cat, configs)
	out := Overview{
		PanelGuid:   cat.PanelGuid,
		Timezone:    cat.Timezone,
		GeneratedAt: time.Now().Unix(),
		Nodes:       []NodeOverview{},
		Policies:    []PolicyOverview{},
	}
	for _, n := range cat.Nodes {
		out.Nodes = append(out.Nodes, NodeOverview{
			CapacityBytes: configs[n.Key].CapacityBytes, AllocatedBytes: allocated[n.Key], ResetSchedule: configs[n.Key].ResetSchedule,
			Key:           n.Key,
			ID:            n.ID,
			Name:          n.Name,
			Guid:          n.Guid,
			Enabled:       n.Enabled,
			Status:        n.Status,
			IsLocal:       n.IsLocal,
			DefaultRatio:  defaults[n.Key],
			DefaultWeight: configs[n.Key].DefaultWeight,
		})
	}
	nodeName := make(map[string]string, len(cat.Nodes))
	for _, n := range cat.Nodes {
		nodeName[n.Key] = n.Name
	}
	seen := make(map[string]bool, len(deliveries))
	for _, d := range deliveries {
		if d.Remove && d.Status == DeliveryRemoved {
			continue
		}
		seen[fmt.Sprintf("%s|%d|%s", d.NodeKey, d.ClientID, cat.PanelGuid)] = true
		view := PolicyOverview{
			NodeKey:      d.NodeKey,
			NodeName:     nodeName[d.NodeKey],
			ClientID:     d.ClientID,
			ClientEmail:  d.ClientEmail,
			Revision:     d.Revision,
			Status:       d.Status,
			LastError:    d.LastError,
			LastReportAt: d.LastReportAt,
			ResetPending: d.ResetPending,
			Remove:       d.Remove,
		}
		if d.LastReportRaw != "" {
			var report PolicyReport
			if json.Unmarshal([]byte(d.LastReportRaw), &report) == nil {
				view.MasterGuid = report.MasterGuid
				view.State = report.State
				view.BudgetBytes = report.BudgetBytes
				view.EffectiveRatio = report.EffectiveRatio
				view.UsageUp = report.UsageUp
				view.UsageDown = report.UsageDown
				view.PeriodStart = report.PeriodStart
				view.PeriodEnd = report.PeriodEnd
				view.GraceStartedAt = report.GraceStartedAt
				view.GraceDeadline = report.GraceDeadline
				view.GraceExtraBytes = report.GraceExtraBytes
				view.GatewayPort = report.GatewayPort
				view.GatewayListening = report.GatewayListening
				view.ActiveConnections = report.ActiveConnections
				view.ManagedInbounds = report.ManagedInbounds
				view.Applied = report.Applied
				view.ApplyError = report.ApplyError
				view.DisabledAt = report.DisabledAt
			}
		}
		out.Policies = append(out.Policies, view)
	}
	for _, row := range applied {
		if row.Removed {
			continue
		}
		if seen[fmt.Sprintf("%s|%d|%s", LocalNodeKey, row.ClientID, row.MasterGuid)] {
			continue
		}
		report := reportFromRow(row)
		out.Policies = append(out.Policies, PolicyOverview{
			MasterGuid:      row.MasterGuid,
			NodeKey:         LocalNodeKey,
			NodeName:        nodeName[LocalNodeKey],
			ClientID:        row.ClientID,
			ClientEmail:     row.ClientEmail,
			Revision:        row.Revision,
			Status:          DeliveryApplied,
			State:           report.State,
			BudgetBytes:     report.BudgetBytes,
			EffectiveRatio:  report.EffectiveRatio,
			UsageUp:         report.UsageUp,
			UsageDown:       report.UsageDown,
			PeriodStart:     report.PeriodStart,
			PeriodEnd:       report.PeriodEnd,
			GraceStartedAt:  report.GraceStartedAt,
			GraceDeadline:   report.GraceDeadline,
			GraceExtraBytes: report.GraceExtraBytes,
			GatewayPort:     report.GatewayPort,
			ManagedInbounds: report.ManagedInbounds,
			Applied:         report.Applied,
			ApplyError:      report.ApplyError,
			DisabledAt:      report.DisabledAt,
			Remote:          row.MasterGuid != cat.PanelGuid,
		})
	}
	sort.Slice(out.Policies, func(i, j int) bool {
		if out.Policies[i].NodeKey != out.Policies[j].NodeKey {
			return out.Policies[i].NodeKey < out.Policies[j].NodeKey
		}
		if out.Policies[i].ClientID != out.Policies[j].ClientID {
			return out.Policies[i].ClientID < out.Policies[j].ClientID
		}
		return out.Policies[i].MasterGuid < out.Policies[j].MasterGuid
	})
	if rt := GetRuntime(); rt != nil {
		out.Sync = rt.SyncStatus()
	}
	return out, nil
}
