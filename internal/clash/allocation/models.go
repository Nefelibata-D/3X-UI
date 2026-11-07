package allocation

import (
	"encoding/json"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
)

const (
	ModeInherit = "inherit"
	ModeCustom  = "custom"
)

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
