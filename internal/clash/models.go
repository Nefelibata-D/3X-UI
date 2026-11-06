package clash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
)

const (
	ModeInherit = "inherit"
	ModeCustom  = "custom"

	DeliveryPending = "pending"
	DeliveryApplied = "applied"
	DeliveryError   = "error"
	DeliveryOffline = "offline"
	DeliveryStale   = "stale"
	DeliveryRemoved = "removed"

	LocalNodeKey = "local"

	DefaultGraceSeconds = int64(600)
	DefaultGracePercent = int64(10)
)

func NodeKey(nodeID int) string { return fmt.Sprintf("node:%d", nodeID) }

type NodeWeight struct {
	CapacityBytes int64                 `gorm:"column:capacity_bytes;not null;default:0" json:"capacityBytes"`
	ResetSchedule gateway.ResetSchedule `gorm:"embedded;embeddedPrefix:quota_reset_" json:"resetSchedule"`
	DefaultWeight int64                 `gorm:"column:default_weight;not null;default:1" json:"defaultWeight"`
	ID            int                   `gorm:"primaryKey;autoIncrement" json:"-"`
	NodeKey       string                `gorm:"column:node_key;uniqueIndex;not null" json:"nodeKey"`
	NodeID        *int                  `gorm:"column:node_id" json:"nodeId,omitempty"`
	NodeGuid      string                `gorm:"column:node_guid" json:"nodeGuid"`
	CreatedAt     int64                 `gorm:"autoCreateTime:milli" json:"createdAt"`
	UpdatedAt     int64                 `gorm:"autoUpdateTime:milli" json:"updatedAt"`
}

func (NodeWeight) TableName() string { return "clash_node_budgets" }

type ClientRatio struct {
	ResetMode  string `gorm:"column:reset_mode;not null;default:node" json:"resetMode"`
	ID         int    `gorm:"primaryKey;autoIncrement" json:"-"`
	ClientID   int    `gorm:"column:client_id;uniqueIndex;not null" json:"clientId"`
	Email      string `gorm:"column:email" json:"email"`
	Mode       string `gorm:"column:mode;not null;default:inherit" json:"mode"`
	WeightsRaw string `gorm:"column:weights_json;type:text" json:"-"`
	LimitsRaw  string `gorm:"column:limits_json;type:text" json:"-"`
	CreatedAt  int64  `gorm:"autoCreateTime:milli" json:"createdAt"`
	UpdatedAt  int64  `gorm:"autoUpdateTime:milli" json:"updatedAt"`
}

func (ClientRatio) TableName() string { return "clash_client_ratios" }

func (r ClientRatio) Weights() map[string]int64 {
	out := map[string]int64{}
	if r.WeightsRaw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(r.WeightsRaw), &out)
	return out
}

func (r ClientRatio) Limits() map[string]int64 {
	out := map[string]int64{}
	if r.LimitsRaw != "" {
		_ = json.Unmarshal([]byte(r.LimitsRaw), &out)
	}
	return out
}

func EncodeWeights(weights map[string]int64) (string, error) {
	if weights == nil {
		return "{}", nil
	}
	b, err := json.Marshal(weights)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type Delivery struct {
	ID            int    `gorm:"primaryKey;autoIncrement" json:"-"`
	NodeKey       string `gorm:"column:node_key;uniqueIndex:idx_clash_delivery" json:"nodeKey"`
	ClientID      int    `gorm:"column:client_id;uniqueIndex:idx_clash_delivery" json:"clientId"`
	ClientEmail   string `gorm:"column:client_email" json:"clientEmail"`
	TargetGuid    string `gorm:"column:target_guid" json:"targetGuid"`
	Revision      int64  `gorm:"column:revision;not null;default:0" json:"revision"`
	PayloadHash   string `gorm:"column:payload_hash" json:"payloadHash"`
	ResetPending  bool   `gorm:"column:reset_pending;default:false" json:"resetPending"`
	Status        string `gorm:"column:status" json:"status"`
	LastError     string `gorm:"column:last_error" json:"lastError"`
	LastReportRaw string `gorm:"column:last_report_json;type:text" json:"-"`
	LastReportAt  int64  `gorm:"column:last_report_at" json:"lastReportAt"`
	Remove        bool   `gorm:"column:remove;default:false" json:"remove"`
	CreatedAt     int64  `gorm:"autoCreateTime:milli" json:"createdAt"`
	UpdatedAt     int64  `gorm:"autoUpdateTime:milli" json:"updatedAt"`
}

func (Delivery) TableName() string { return "clash_deliveries" }

type AppliedPolicy struct {
	LedgerResetSchedule  gateway.ResetSchedule `gorm:"embedded;embeddedPrefix:ledger_reset_" json:"-"`
	LedgerPeriodTimezone string                `json:"-"`
	AllocationRequired   bool                  `json:"allocationRequired"`
	ResetSchedule        gateway.ResetSchedule `gorm:"embedded;embeddedPrefix:quota_reset_" json:"resetSchedule"`
	ID                   int                   `gorm:"primaryKey;autoIncrement" json:"-"`
	MasterGuid           string                `gorm:"column:master_guid;uniqueIndex:idx_clash_applied" json:"masterGuid"`
	ClientID             int                   `gorm:"column:client_id;uniqueIndex:idx_clash_applied" json:"clientId"`
	ClientEmail          string                `gorm:"column:client_email" json:"clientEmail"`
	Revision             int64                 `gorm:"column:revision;not null;default:0" json:"revision"`
	BudgetBytes          int64                 `gorm:"column:budget_bytes" json:"budgetBytes"`
	GraceSeconds         int64                 `gorm:"column:grace_seconds" json:"graceSeconds"`
	GracePercent         int64                 `gorm:"column:grace_percent" json:"gracePercent"`
	ActivitySeconds      int64                 `gorm:"column:activity_seconds;default:0" json:"activitySeconds"`
	SiteGroupsJSON       string                `gorm:"column:site_groups_json;type:text" json:"-"`
	EffectiveRatio       float64               `gorm:"column:effective_ratio" json:"effectiveRatio"`
	PeriodTimezone       string                `gorm:"column:period_timezone" json:"periodTimezone"`
	GatewayPort          int                   `gorm:"column:gateway_port" json:"gatewayPort"`
	ManagedJSON          string                `gorm:"column:managed_json;type:text" json:"-"`
	State                string                `gorm:"column:state" json:"state"`
	UsageUp              int64                 `gorm:"column:usage_up" json:"usageUp"`
	UsageDown            int64                 `gorm:"column:usage_down" json:"usageDown"`
	PeriodStart          int64                 `gorm:"column:period_start" json:"periodStart"`
	PeriodEnd            int64                 `gorm:"column:period_end" json:"periodEnd"`
	GraceStartedAt       int64                 `gorm:"column:grace_started_at" json:"graceStartedAt"`
	GraceDeadline        int64                 `gorm:"column:grace_deadline" json:"graceDeadline"`
	GraceExtraBytes      int64                 `gorm:"column:grace_extra_bytes" json:"graceExtraBytes"`
	GraceBaseline        int64                 `gorm:"column:grace_baseline" json:"graceBaseline"`
	GraceGroupsJSON      string                `gorm:"column:grace_groups_json;type:text" json:"-"`
	GraceSnapshotRaw     string                `gorm:"column:grace_snapshot_json;type:text" json:"-"`
	DisabledAt           int64                 `gorm:"column:disabled_at" json:"disabledAt"`
	ResetAppliedRevision int64                 `gorm:"column:reset_applied_revision;default:0" json:"-"`
	Applied              bool                  `gorm:"column:applied;default:false" json:"applied"`
	ApplyError           string                `gorm:"column:apply_error" json:"applyError"`
	Removing             bool                  `gorm:"column:removing;default:false" json:"removing"`
	Removed              bool                  `gorm:"column:removed;default:false" json:"removed"`
	CreatedAt            int64                 `gorm:"autoCreateTime:milli" json:"createdAt"`
	UpdatedAt            int64                 `gorm:"autoUpdateTime:milli" json:"updatedAt"`
}

func (AppliedPolicy) TableName() string { return "clash_applied_policies" }

func (p AppliedPolicy) ManagedInbounds() []ManagedInbound {
	if p.ManagedJSON == "" {
		return nil
	}
	var out []ManagedInbound
	_ = json.Unmarshal([]byte(p.ManagedJSON), &out)
	return out
}

func (p AppliedPolicy) GraceSnapshot() []string {
	if p.GraceSnapshotRaw == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(p.GraceSnapshotRaw), &out)
	return out
}

func (p AppliedPolicy) GraceGroups() []gateway.SiteGroup {
	var groups []gateway.SiteGroup
	_ = json.Unmarshal([]byte(p.GraceGroupsJSON), &groups)
	return groups
}

func (p AppliedPolicy) SiteGroups() []gateway.SiteGroup {
	if p.SiteGroupsJSON == "" {
		return nil
	}
	var out []gateway.SiteGroup
	_ = json.Unmarshal([]byte(p.SiteGroupsJSON), &out)
	return out
}

func EncodeSiteGroups(groups []gateway.SiteGroup) string {
	if len(groups) == 0 {
		return ""
	}
	b, _ := json.Marshal(groups)
	return string(b)
}

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

// ManagedInbound is the child-side description of an official inbound that
// carries a managed client. It is recomputed from official data on every
// reconcile; only DisabledByGateway and the port check are project state.
type ManagedInbound struct {
	ID                int    `json:"id"`
	Tag               string `json:"tag"`
	Protocol          string `json:"protocol"`
	Listen            string `json:"listen"`
	Port              int    `json:"port"`
	Network           string `json:"network"`
	Security          string `json:"security"`
	Enabled           bool   `json:"enabled"`
	DisabledByGateway bool   `json:"disabledByGateway"`
	PortCheck         string `json:"portCheck,omitempty"`
	PortError         string `json:"portError,omitempty"`
}

type DesiredPolicy struct {
	ResetSchedule      gateway.ResetSchedule `json:"resetSchedule"`
	AllocationRequired bool                  `json:"allocationRequired"`
	MasterGuid         string                `json:"masterGuid"`
	NodeKey            string                `json:"nodeKey"`
	NodeGuid           string                `json:"targetGuid"`
	NodeEnabled        bool                  `json:"nodeEnabled"`
	ClientID           int                   `json:"clientId"`
	ClientEmail        string                `json:"clientEmail"`
	Revision           int64                 `json:"revision"`
	BudgetBytes        int64                 `json:"budgetBytes"`
	GraceSeconds       int64                 `json:"graceSeconds"`
	GracePercent       int64                 `json:"gracePercent"`
	ActivitySeconds    int64                 `json:"activitySeconds,omitempty"`
	SiteGroups         []gateway.SiteGroup   `json:"siteGroups,omitempty"`
	EffectiveRatio     float64               `json:"effectiveRatio"`
	PeriodTimezone     string                `json:"periodTimezone"`
	Reset              bool                  `json:"reset"`
	Remove             bool                  `json:"remove"`
	PayloadHash        string                `json:"payloadHash"`
	Status             string                `json:"status"`
	LastError          string                `json:"lastError"`
	ExistingID         int                   `json:"-"`
}

type SyncPlan struct {
	Policies []DesiredPolicy `json:"policies"`
	Removals []DesiredPolicy `json:"removals"`
}

// EffectiveWeights resolves a client's weights inside its authorized,
// selectable node set. Missing custom weights are zero; inherit uses explicitly configured node weights.
func EffectiveWeights(cat *Catalog, clientID int, nodes map[string]NodeWeight, ratio *ClientRatio) ([]RatioResult, error) {
	authorized := cat.AuthorizedNodeKeys(clientID)
	inputs := make([]WeightInput, 0, len(authorized))
	for _, key := range authorized {
		node, ok := cat.NodeByKey(key)
		if !ok || !node.Enabled {
			continue
		}
		weight := nodes[key].DefaultWeight
		if ratio != nil && ratio.Mode == ModeCustom {
			weight = ratio.Weights()[key]
		}
		inputs = append(inputs, WeightInput{Key: key, Weight: weight})
	}
	if len(inputs) == 0 {
		return nil, ErrNoSelectableOutlet
	}
	return NormalizeWeights(inputs)
}

var (
	ErrNoSelectableOutlet = errors.New("no selectable outlet")
	ErrInvalidMode        = errors.New("ratio mode must be inherit or custom")
)

func ValidateMode(mode string) error {
	if mode != ModeInherit && mode != ModeCustom {
		return ErrInvalidMode
	}
	return nil
}

func ValidateWeights(weights map[string]int64) error {
	for _, w := range weights {
		if w < 0 {
			return ErrNegativeWeight
		}
	}
	return nil
}

// PlanDeliveries compares the desired policy set with the stored deliveries.
// It allocates a new revision only when the payload changes or a reset was
// requested, so retries are idempotent and an older revision can never replace
// a newer one on the child.
func PlanDeliveries(cat *Catalog, nodes map[string]NodeWeight, ratios map[int]ClientRatio, existing []Delivery, graceSeconds, gracePercent int64) (SyncPlan, error) {
	byKey := make(map[string]Delivery, len(existing))
	for _, d := range existing {
		byKey[deliveryKey(d.NodeKey, d.ClientID)] = d
	}
	desired := map[string]DesiredPolicy{}
	for _, node := range cat.Nodes {
		for _, client := range cat.ClientsOnNode(node.Key) {
			authorized := cat.AuthorizedNodeKeys(client.ID)
			if len(authorized) == 0 {
				continue
			}
			ratio := ratios[client.ID]
			budget := ratio.Limits()[node.Key]
			var ratioPtr *ClientRatio
			if ratio.ClientID != 0 {
				ratioPtr = &ratio
			}
			weights, err := EffectiveWeights(cat, client.ID, nodes, ratioPtr)
			if err != nil {
				if !errors.Is(err, ErrNoSelectableOutlet) && !errors.Is(err, ErrZeroWeightSum) {
					return SyncPlan{}, fmt.Errorf("client %d: %w", client.ID, err)
				}
				// A client attached only to a temporarily disabled node keeps
				// its last policy; removal would make the child unrecoverable
				// while the node is unreachable. Enabled nodes with a positive
				// budget always contribute the target node itself.
				if node.Enabled && errors.Is(err, ErrNoSelectableOutlet) {
					continue
				}
				weights = nil
			}
			var effective float64
			for _, w := range weights {
				if w.Key == node.Key {
					effective = w.Ratio
					break
				}
			}
			schedule := nodes[node.Key].ResetSchedule
			schedule.Disabled = ratio.ResetMode == "manual" || (client.TrafficReset != "" && client.TrafficReset != "never")
			payload := DesiredPolicy{
				ResetSchedule: schedule, AllocationRequired: nodes[node.Key].CapacityBytes > 0,
				MasterGuid:     cat.PanelGuid,
				NodeKey:        node.Key,
				NodeGuid:       node.Guid,
				NodeEnabled:    node.Enabled,
				ClientID:       client.ID,
				ClientEmail:    client.Email,
				BudgetBytes:    budget,
				GraceSeconds:   graceSeconds,
				GracePercent:   gracePercent,
				EffectiveRatio: effective,
				PeriodTimezone: cat.Timezone,
			}
			key := deliveryKey(node.Key, client.ID)
			old, exists := byKey[key]
			payload.ExistingID = old.ID
			payload.PayloadHash = payloadHash(payload)
			payload.Revision = 1
			payload.Status = DeliveryPending
			if exists {
				payload.Revision = old.Revision
				payload.Status = old.Status
				payload.LastError = old.LastError
				if old.PayloadHash != payload.PayloadHash {
					if payload.Revision < 1 {
						payload.Revision = 1
					}
					payload.Revision++
					payload.Status = DeliveryPending
					payload.LastError = ""
				}
				payload.Reset = old.ResetPending
				if old.Remove {
					payload.Revision++
					payload.Status = DeliveryPending
					payload.Reset = false
					payload.LastError = ""
				}
			}
			desired[key] = payload
		}
	}
	plan := SyncPlan{}
	for _, p := range desired {
		plan.Policies = append(plan.Policies, p)
	}
	sort.Slice(plan.Policies, func(i, j int) bool {
		if plan.Policies[i].NodeKey != plan.Policies[j].NodeKey {
			return plan.Policies[i].NodeKey < plan.Policies[j].NodeKey
		}
		return plan.Policies[i].ClientID < plan.Policies[j].ClientID
	})
	for _, old := range existing {
		if _, ok := desired[deliveryKey(old.NodeKey, old.ClientID)]; ok {
			continue
		}
		if old.Remove && old.Status == DeliveryRemoved {
			continue
		}
		removal := DesiredPolicy{
			MasterGuid:     cat.PanelGuid,
			NodeKey:        old.NodeKey,
			NodeGuid:       old.TargetGuid,
			ClientID:       old.ClientID,
			ClientEmail:    old.ClientEmail,
			Revision:       old.Revision + 1,
			PeriodTimezone: cat.Timezone,
			Remove:         true,
			Status:         DeliveryPending,
			ExistingID:     old.ID,
		}
		if old.Remove {
			removal.Revision = old.Revision
		}
		if node, ok := cat.NodeByKey(old.NodeKey); ok {
			removal.NodeGuid = node.Guid
			removal.NodeEnabled = node.Enabled
		}
		removal.PayloadHash = payloadHash(removal)
		plan.Removals = append(plan.Removals, removal)
	}
	sort.Slice(plan.Removals, func(i, j int) bool {
		if plan.Removals[i].NodeKey != plan.Removals[j].NodeKey {
			return plan.Removals[i].NodeKey < plan.Removals[j].NodeKey
		}
		return plan.Removals[i].ClientID < plan.Removals[j].ClientID
	})
	return plan, nil
}

func deliveryKey(nodeKey string, clientID int) string {
	return fmt.Sprintf("%s|%d", nodeKey, clientID)
}

func payloadHash(p DesiredPolicy) string {
	payload := struct {
		ResetSchedule      gateway.ResetSchedule `json:"resetSchedule"`
		AllocationRequired bool                  `json:"allocationRequired"`
		MasterGuid         string                `json:"masterGuid"`
		TargetGuid         string                `json:"targetGuid"`
		ClientID           int                   `json:"clientId"`
		ClientEmail        string                `json:"clientEmail"`
		BudgetBytes        int64                 `json:"budgetBytes"`
		GraceSeconds       int64                 `json:"graceSeconds"`
		GracePercent       int64                 `json:"gracePercent"`
		ActivitySeconds    int64                 `json:"activitySeconds"`
		SiteGroups         []gateway.SiteGroup   `json:"siteGroups"`
		EffectiveRatio     float64               `json:"effectiveRatio"`
		PeriodTimezone     string                `json:"periodTimezone"`
		Remove             bool                  `json:"remove"`
	}{
		ResetSchedule: p.ResetSchedule, AllocationRequired: p.AllocationRequired,
		MasterGuid:      p.MasterGuid,
		TargetGuid:      p.NodeGuid,
		ClientID:        p.ClientID,
		ClientEmail:     p.ClientEmail,
		BudgetBytes:     p.BudgetBytes,
		GraceSeconds:    p.GraceSeconds,
		GracePercent:    p.GracePercent,
		ActivitySeconds: p.ActivitySeconds,
		SiteGroups:      p.SiteGroups,
		EffectiveRatio:  p.EffectiveRatio,
		PeriodTimezone:  p.PeriodTimezone,
		Remove:          p.Remove,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func RatioMap(rows []ClientRatio) map[int]ClientRatio {
	out := make(map[int]ClientRatio, len(rows))
	for _, row := range rows {
		out[row.ClientID] = row
	}
	return out
}

func nowMillis() int64 { return time.Now().UnixMilli() }

// Unconfigured outlets start with equal weight and no project quota.
func NodeConfigMap(cat *Catalog, rows []NodeWeight) map[string]NodeWeight {
	out := make(map[string]NodeWeight, len(cat.Nodes))
	for _, n := range cat.Nodes {
		out[n.Key] = NodeWeight{NodeKey: n.Key, DefaultWeight: 1}
	}
	for _, row := range rows {
		out[row.NodeKey] = row
	}
	return out
}
