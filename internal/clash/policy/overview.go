package policy

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/allocation"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
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

func BuildOverview(cat *catalog.Catalog) (Overview, error) {
	nodeWeights, err := allocation.ListNodeWeights()
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
	configs := allocation.NodeConfigMap(cat, nodeWeights)
	ratios, err := allocation.ListRatios()
	if err != nil {
		return Overview{}, err
	}
	allocated := allocation.NodeAllocations(cat, allocation.RatioMap(ratios))
	defaults := allocation.NodeDefaultRatios(cat, configs)
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
		if seen[fmt.Sprintf("%s|%d|%s", catalog.LocalNodeKey, row.ClientID, row.MasterGuid)] {
			continue
		}
		report := reportFromRow(row)
		out.Policies = append(out.Policies, PolicyOverview{
			MasterGuid:      row.MasterGuid,
			NodeKey:         catalog.LocalNodeKey,
			NodeName:        nodeName[catalog.LocalNodeKey],
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
