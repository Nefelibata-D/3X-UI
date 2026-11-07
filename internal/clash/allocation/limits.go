package allocation

import (
	"fmt"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
)

func NodeAllocations(cat *catalog.Catalog, ratios map[int]ClientRatio) map[string]int64 {
	out := map[string]int64{}
	for _, client := range cat.Clients {
		limits := ratios[client.ID].Limits()
		for _, key := range cat.AuthorizedNodeKeys(client.ID) {
			out[key] += limits[key]
		}
	}
	return out
}

func ValidateNodeAllocations(cat *catalog.Catalog, nodes map[string]NodeWeight, ratios map[int]ClientRatio, requiredClientID int) error {
	for key, node := range nodes {
		if node.CapacityBytes <= 0 {
			continue
		}
		remaining := node.CapacityBytes
		for _, client := range cat.ClientsOnNode(key) {
			limit := ratios[client.ID].Limits()[key]
			if limit <= 0 && (requiredClientID < 0 || requiredClientID == client.ID) {
				return fmt.Errorf("节点 %s 总额度有限，请先为客户端 %s 分配明确额度", key, client.Email)
			}
			if limit > remaining {
				return fmt.Errorf("节点 %s 的客户端分配合计超过节点总额度", key)
			}
			remaining -= limit
		}
	}
	return nil
}
