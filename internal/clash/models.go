package clash

import (
	"encoding/json"
	"fmt"

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

type ManagedInbound struct {
	ID       int    `json:"id"`
	Tag      string `json:"tag"`
	Protocol string `json:"protocol"`
	Listen   string `json:"listen"`
	Port     int    `json:"port"`
	Network  string `json:"network"`
	Security string `json:"security"`
	Enabled  bool   `json:"enabled"`
}
