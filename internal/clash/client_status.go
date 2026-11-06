package clash

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"
)

// ClientStatus is the Mihomo wire contract. Only explicit policy states yield
// 200/409/403; unavailable policy reports are transport errors, not permission.
func ClientStatus(ctx context.Context, credential, nodeID string) int {
	cat, err := LoadCatalogOrError()
	if err != nil {
		return http.StatusServiceUnavailable
	}
	var client *ClientInfo
	for i := range cat.Clients {
		c := &cat.Clients[i]
		if credential != "" && c.UUID != "" && subtle.ConstantTimeCompare([]byte(credential), []byte(c.UUID)) == 1 {
			client = c
			break
		}
	}
	if client == nil {
		return http.StatusUnauthorized
	}
	var node *NodeInfo
	for _, key := range cat.AuthorizedNodeKeys(client.ID) {
		n, ok := cat.NodeByKey(key)
		if ok && n.Guid == nodeID {
			node = &n
			break
		}
	}
	if node == nil {
		return http.StatusNotFound
	}
	if !client.Enabled || (client.ExpiryTime > 0 && client.ExpiryTime <= time.Now().UnixMilli()) {
		return http.StatusForbidden
	}
	if !node.Enabled {
		return http.StatusServiceUnavailable
	}
	d, err := GetDelivery(node.Key, client.ID)
	if err != nil || d.Remove {
		return http.StatusServiceUnavailable
	}
	var report PolicyReport
	if node.IsLocal {
		rt := GetRuntime()
		if rt == nil {
			return http.StatusServiceUnavailable
		}
		report, err = rt.Report(cat.PanelGuid, client.ID)
	} else {
		n, e := loadNodeForDelivery(node.Key)
		if e != nil {
			return http.StatusServiceUnavailable
		}
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		report, err = reportNode(requestCtx, n, cat.PanelGuid, client.ID)
	}
	if err != nil || report.Revision != d.Revision || report.Removed || report.Removing || report.LastError != "" || !report.Applied {
		return http.StatusServiceUnavailable
	}
	switch report.State {
	case "normal":
		return http.StatusOK
	case "grace":
		return http.StatusConflict
	case "disabled":
		return http.StatusForbidden
	default:
		return http.StatusServiceUnavailable
	}
}
