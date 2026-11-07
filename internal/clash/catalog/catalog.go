package catalog

import (
	"sort"
)

type NodeInfo struct {
	ID      int    `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Guid    string `json:"guid"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	IsLocal bool   `json:"isLocal"`
}

type InboundInfo struct {
	ID       int      `json:"id"`
	NodeKey  string   `json:"nodeKey"`
	Tag      string   `json:"tag"`
	Protocol string   `json:"protocol"`
	Listen   string   `json:"listen"`
	Port     int      `json:"port"`
	Network  string   `json:"network"`
	Security string   `json:"security"`
	Enabled  bool     `json:"enabled"`
	Clients  []string `json:"clients"`
}

type ClientInfo struct {
	TotalBytes   int64  `json:"totalBytes"`
	TrafficReset string `json:"trafficReset"`
	ID           int    `json:"id"`
	Email        string `json:"email"`
	UUID         string `json:"-"`
	ExpiryTime   int64  `json:"-"`
	Enabled      bool   `json:"enabled"`
	InboundIDs   []int  `json:"inboundIds"`
}

type Catalog struct {
	PanelGuid string        `json:"panelGuid"`
	Timezone  string        `json:"timezone"`
	Nodes     []NodeInfo    `json:"nodes"`
	Inbounds  []InboundInfo `json:"inbounds"`
	Clients   []ClientInfo  `json:"clients"`
}

func (c *Catalog) NodeByKey(key string) (NodeInfo, bool) {
	for _, n := range c.Nodes {
		if n.Key == key {
			return n, true
		}
	}
	return NodeInfo{}, false
}

func (c *Catalog) ClientByID(id int) (ClientInfo, bool) {
	for _, cl := range c.Clients {
		if cl.ID == id {
			return cl, true
		}
	}
	return ClientInfo{}, false
}

func (c *Catalog) ClientByEmail(email string) (ClientInfo, bool) {
	for _, cl := range c.Clients {
		if cl.Email == email {
			return cl, true
		}
	}
	return ClientInfo{}, false
}

func (c *Catalog) InboundByID(id int) (InboundInfo, bool) {
	for _, in := range c.Inbounds {
		if in.ID == id {
			return in, true
		}
	}
	return InboundInfo{}, false
}

// AuthorizedNodeKeys returns the distinct node keys of a client's attached
// inbounds. Inbounds on unknown/deleted nodes are ignored.
func (c *Catalog) AuthorizedNodeKeys(clientID int) []string {
	client, ok := c.ClientByID(clientID)
	if !ok {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(client.InboundIDs))
	for _, id := range client.InboundIDs {
		in, ok := c.InboundByID(id)
		if !ok {
			continue
		}
		if _, ok := c.NodeByKey(in.NodeKey); !ok {
			continue
		}
		if _, dup := seen[in.NodeKey]; dup {
			continue
		}
		seen[in.NodeKey] = struct{}{}
		out = append(out, in.NodeKey)
	}
	sort.Strings(out)
	return out
}

// ClientsOnNode returns every client attached to at least one inbound hosted
// by nodeKey.
func (c *Catalog) ClientsOnNode(nodeKey string) []ClientInfo {
	out := make([]ClientInfo, 0)
	for _, cl := range c.Clients {
		if cl.ID <= 0 {
			// A child-side settings-only client has no master client id and
			// cannot participate in ratio planning.
			continue
		}
		for _, id := range cl.InboundIDs {
			in, ok := c.InboundByID(id)
			if ok && in.NodeKey == nodeKey {
				out = append(out, cl)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
