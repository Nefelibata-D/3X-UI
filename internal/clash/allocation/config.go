package allocation

import (
	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
)

func RatioMap(rows []ClientRatio) map[int]ClientRatio {
	out := make(map[int]ClientRatio, len(rows))
	for _, row := range rows {
		out[row.ClientID] = row
	}
	return out
}

// Unconfigured outlets start with equal weight and no project quota.
func NodeConfigMap(cat *catalog.Catalog, rows []NodeWeight) map[string]NodeWeight {
	out := make(map[string]NodeWeight, len(cat.Nodes))
	for _, n := range cat.Nodes {
		out[n.Key] = NodeWeight{NodeKey: n.Key, DefaultWeight: 1}
	}
	for _, row := range rows {
		out[row.NodeKey] = row
	}
	return out
}
