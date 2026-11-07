package service

import (
	"encoding/json"

	"github.com/mhsanaei/3x-ui/v3/internal/clash"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// NewClashCatalogProvider builds the project's read-only view of official
// nodes, inbounds and clients. Official rows are read through their services;
// quota enforcement never modifies the upstream inbound enabled state.
func NewClashCatalogProvider() clash.CatalogProvider {
	return func() (*clash.Catalog, error) {
		settingSvc := &SettingService{}
		guid, err := settingSvc.GetPanelGuid()
		if err != nil {
			return nil, err
		}
		loc, err := settingSvc.GetTimeLocation()
		if err != nil {
			return nil, err
		}
		nodes, err := (&NodeService{}).GetAll()
		if err != nil {
			return nil, err
		}
		inbounds, err := (&InboundService{}).GetAllInbounds()
		if err != nil {
			return nil, err
		}
		clients, err := (&ClientService{}).List()
		if err != nil {
			return nil, err
		}

		nodeByID := make(map[int]*model.Node, len(nodes))
		catalogNodes := []clash.NodeInfo{{
			ID:      0,
			Key:     clash.LocalNodeKey,
			Name:    "local",
			Guid:    guid,
			Enabled: true,
			Status:  "online",
			IsLocal: true,
		}}
		for _, n := range nodes {
			nodeByID[n.Id] = n
			catalogNodes = append(catalogNodes, clash.NodeInfo{
				ID:      n.Id,
				Key:     clash.NodeKey(n.Id),
				Name:    n.Name,
				Guid:    n.Guid,
				Enabled: n.Enable,
				Status:  n.Status,
			})
		}

		emailsByInbound := make(map[int][]string, len(inbounds))
		catalogClients := make([]clash.ClientInfo, 0, len(clients))
		clientIndex := make(map[string]int, len(clients))
		addEmail := func(inboundID int, email string) {
			if email == "" {
				return
			}
			for _, existing := range emailsByInbound[inboundID] {
				if existing == email {
					return
				}
			}
			emailsByInbound[inboundID] = append(emailsByInbound[inboundID], email)
		}
		attachInbound := func(idx int, inboundID int) {
			for _, existing := range catalogClients[idx].InboundIDs {
				if existing == inboundID {
					return
				}
			}
			catalogClients[idx].InboundIDs = append(catalogClients[idx].InboundIDs, inboundID)
		}
		for _, cl := range clients {
			clientIndex[cl.Email] = len(catalogClients)
			catalogClients = append(catalogClients, clash.ClientInfo{
				ID:           cl.Id,
				UUID:         cl.UUID,
				TotalBytes:   cl.TotalGB,
				TrafficReset: cl.TrafficReset,
				ExpiryTime:   cl.ExpiryTime,
				Email:        cl.Email,
				Enabled:      cl.Enable,
				InboundIDs:   append([]int(nil), cl.InboundIds...),
			})
			for _, inboundID := range cl.InboundIds {
				addEmail(inboundID, cl.Email)
			}
		}
		// Node-synced inbounds can carry a client in settings without a local
		// clients row; Xray still serves it, so the project must see it too.
		for _, ib := range inbounds {
			var settings struct {
				Clients []struct {
					Email string `json:"email"`
				} `json:"clients"`
			}
			if json.Unmarshal([]byte(ib.Settings), &settings) != nil {
				continue
			}
			for _, client := range settings.Clients {
				if client.Email == "" {
					continue
				}
				addEmail(ib.Id, client.Email)
				if idx, ok := clientIndex[client.Email]; ok {
					attachInbound(idx, ib.Id)
				} else {
					clientIndex[client.Email] = len(catalogClients)
					catalogClients = append(catalogClients, clash.ClientInfo{
						ID:         0,
						Email:      client.Email,
						Enabled:    true,
						InboundIDs: []int{ib.Id},
					})
				}
			}
		}

		catalogInbounds := make([]clash.InboundInfo, 0, len(inbounds))
		for _, ib := range inbounds {
			key := clash.LocalNodeKey
			if ib.NodeID != nil {
				if _, ok := nodeByID[*ib.NodeID]; !ok {
					continue
				}
				key = clash.NodeKey(*ib.NodeID)
			}
			var stream struct {
				Network  string `json:"network"`
				Security string `json:"security"`
			}
			_ = json.Unmarshal([]byte(ib.StreamSettings), &stream)
			catalogInbounds = append(catalogInbounds, clash.InboundInfo{
				ID:       ib.Id,
				NodeKey:  key,
				Tag:      ib.Tag,
				Protocol: string(ib.Protocol),
				Listen:   ib.Listen,
				Port:     ib.Port,
				Network:  stream.Network,
				Security: stream.Security,
				Enabled:  ib.Enable,
				Clients:  append([]string(nil), emailsByInbound[ib.Id]...),
			})
		}
		return &clash.Catalog{
			PanelGuid: guid,
			Timezone:  loc.String(),
			Nodes:     catalogNodes,
			Inbounds:  catalogInbounds,
			Clients:   catalogClients,
		}, nil
	}
}
