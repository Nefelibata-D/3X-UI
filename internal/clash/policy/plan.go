package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/allocation"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
)

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

// PlanDeliveries compares the desired policy set with the stored deliveries.
// It allocates a new revision only when the payload changes or a reset was
// requested, so retries are idempotent and an older revision can never replace
// a newer one on the child.
func PlanDeliveries(cat *catalog.Catalog, nodes map[string]allocation.NodeWeight, ratios map[int]allocation.ClientRatio, existing []Delivery, graceSeconds, gracePercent int64) (SyncPlan, error) {
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
			var ratioPtr *allocation.ClientRatio
			if ratio.ClientID != 0 {
				ratioPtr = &ratio
			}
			weights, err := allocation.EffectiveWeights(cat, client.ID, nodes, ratioPtr)
			if err != nil {
				if !errors.Is(err, allocation.ErrNoSelectableOutlet) && !errors.Is(err, allocation.ErrZeroWeightSum) {
					return SyncPlan{}, fmt.Errorf("client %d: %w", client.ID, err)
				}
				// A client attached only to a temporarily disabled node keeps
				// its last policy; removal would make the child unrecoverable
				// while the node is unreachable. Enabled nodes with a positive
				// budget always contribute the target node itself.
				if node.Enabled && errors.Is(err, allocation.ErrNoSelectableOutlet) {
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
		EffectiveRatio     float64               `json:"effectiveRatio"`
		PeriodTimezone     string                `json:"periodTimezone"`
		Remove             bool                  `json:"remove"`
	}{
		ResetSchedule: p.ResetSchedule, AllocationRequired: p.AllocationRequired,
		MasterGuid:     p.MasterGuid,
		TargetGuid:     p.NodeGuid,
		ClientID:       p.ClientID,
		ClientEmail:    p.ClientEmail,
		BudgetBytes:    p.BudgetBytes,
		GraceSeconds:   p.GraceSeconds,
		GracePercent:   p.GracePercent,
		EffectiveRatio: p.EffectiveRatio,
		PeriodTimezone: p.PeriodTimezone,
		Remove:         p.Remove,
	}
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
