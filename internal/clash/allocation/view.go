package allocation

import (
	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
)

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

func NodeDefaultRatios(cat *catalog.Catalog, nodes map[string]NodeWeight) map[string]float64 {
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

func BuildClientRatioView(cat *catalog.Catalog, clientID int, nodes map[string]NodeWeight, ratio *ClientRatio) (ClientRatioView, error) {
	client, ok := cat.ClientByID(clientID)
	if !ok {
		return ClientRatioView{}, catalog.ErrClientNotFound
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
