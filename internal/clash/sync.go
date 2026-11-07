package clash

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

type SyncStatus struct {
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	LastSync int64  `json:"lastSync"`
	Running  bool   `json:"running"`
}

func (rt *Runtime) setSync(status, errText string) {
	rt.statusMu.Lock()
	rt.syncStatus = status
	rt.syncError = errText
	rt.lastSync = time.Now().Unix()
	rt.statusMu.Unlock()
}

func (rt *Runtime) SyncStatus() SyncStatus {
	rt.statusMu.Lock()
	defer rt.statusMu.Unlock()
	return SyncStatus{
		Status:   rt.syncStatus,
		Error:    rt.syncError,
		LastSync: rt.lastSync,
	}
}

// SyncOnce reconciles the master's node weights and per-client node limits into child
// policies. It is safe to call from the API and the background loop; an
// overlapping run is skipped, never queued.
func (rt *Runtime) SyncOnce(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !rt.syncMu.TryLock() {
		return
	}
	defer rt.syncMu.Unlock()

	cat, err := rt.catalogNow()
	if err != nil {
		rt.setSync("error", err.Error())
		return
	}
	nodeWeights, err := ListNodeWeights()
	if err != nil {
		rt.setSync("error", err.Error())
		return
	}
	ratios, err := ListRatios()
	if err != nil {
		rt.setSync("error", err.Error())
		return
	}
	deliveries, err := ListDeliveries()
	if err != nil {
		rt.setSync("error", err.Error())
		return
	}
	plan, err := PlanDeliveries(cat, NodeConfigMap(cat, nodeWeights), RatioMap(ratios), deliveries, DefaultGraceSeconds, DefaultGracePercent)
	if err != nil {
		rt.setSync("error", err.Error())
		return
	}
	desired := make(map[string]DesiredPolicy, len(plan.Policies))
	for _, p := range plan.Policies {
		desired[deliveryKey(p.NodeKey, p.ClientID)] = p
	}
	attempted := make(map[string]bool, len(plan.Policies)+len(plan.Removals))
	for _, p := range plan.Policies {
		rt.syncPolicy(ctx, cat, p, attempted)
	}
	for _, p := range plan.Removals {
		rt.syncRemoval(ctx, cat, p, attempted)
	}
	// Poll every unchanged policy so the master shows the child's current
	// usage/state without making enforcement depend on this poll.
	for _, d := range deliveries {
		key := deliveryKey(d.NodeKey, d.ClientID)
		p, ok := desired[key]
		if !ok || attempted[key] {
			continue
		}
		stored, err := GetDelivery(d.NodeKey, d.ClientID)
		if err != nil {
			continue
		}
		rt.pollDelivery(ctx, cat, p, *stored)
	}
	rt.setSync("ok", "")
}

func (rt *Runtime) syncPolicy(ctx context.Context, cat *Catalog, p DesiredPolicy, attempted map[string]bool) {
	key := deliveryKey(p.NodeKey, p.ClientID)
	stored, err := upsertDesiredDelivery(p)
	if err != nil {
		logger.Warningf("clash: save delivery %s: %v", key, err)
		return
	}
	if stored.Status == DeliveryApplied && !p.Reset && !stored.Remove {
		return
	}
	if !p.NodeEnabled {
		_ = SaveDeliveryReport(stored.ID, DeliveryOffline, "node is disabled", "", p.Reset)
		attempted[key] = true
		return
	}
	if p.NodeGuid == "" {
		_ = SaveDeliveryReport(stored.ID, DeliveryPending, "waiting for node identity (heartbeat)", "", p.Reset)
		attempted[key] = true
		return
	}
	req := ApplyRequest{
		MasterGuid:     cat.PanelGuid,
		TargetGuid:     p.NodeGuid,
		ClientID:       p.ClientID,
		ClientEmail:    p.ClientEmail,
		Revision:       p.Revision,
		BudgetBytes:    p.BudgetBytes,
		GraceSeconds:   p.GraceSeconds,
		GracePercent:   p.GracePercent,
		EffectiveRatio: p.EffectiveRatio,
		ResetSchedule:  p.ResetSchedule, AllocationRequired: p.AllocationRequired,
		PeriodTimezone: p.PeriodTimezone,
		Reset:          p.Reset,
	}
	var (
		report   PolicyReport
		applyErr error
	)
	if p.NodeKey == LocalNodeKey {
		report, applyErr = rt.Apply(req)
	} else {
		node, err := loadNodeForDelivery(p.NodeKey)
		if err != nil {
			applyErr = err
		} else {
			callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			report, applyErr = applyNode(callCtx, node, req)
		}
	}
	attempted[key] = true
	if applyErr != nil {
		status := DeliveryError
		if isStaleError(applyErr) {
			status = DeliveryStale
		} else if isOfflineError(applyErr) {
			status = DeliveryOffline
		}
		_ = SaveDeliveryReport(stored.ID, status, applyErr.Error(), "", p.Reset)
		return
	}
	saveAppliedReport(stored.ID, report, false)
}

func (rt *Runtime) syncRemoval(ctx context.Context, cat *Catalog, p DesiredPolicy, attempted map[string]bool) {
	key := deliveryKey(p.NodeKey, p.ClientID)
	stored, err := upsertDesiredDelivery(p)
	if err != nil {
		logger.Warningf("clash: save removal %s: %v", key, err)
		return
	}
	attempted[key] = true
	if !p.NodeEnabled {
		_ = SaveDeliveryReport(stored.ID, DeliveryOffline, "node is disabled; policy remains on child", "", false)
		return
	}
	if p.NodeGuid == "" && p.NodeKey != LocalNodeKey {
		_ = SaveDeliveryReport(stored.ID, DeliveryPending, "waiting for node identity; policy remains on child", "", false)
		return
	}
	req := ApplyRequest{
		MasterGuid:  cat.PanelGuid,
		TargetGuid:  p.NodeGuid,
		ClientID:    p.ClientID,
		ClientEmail: p.ClientEmail,
		Revision:    p.Revision,
		Remove:      true,
	}
	if p.NodeKey == LocalNodeKey {
		req.TargetGuid = cat.PanelGuid
		_, err = rt.Apply(req)
	} else {
		node, lerr := loadNodeForDelivery(p.NodeKey)
		if lerr != nil {
			err = lerr
		} else {
			callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			_, err = applyNode(callCtx, node, req)
		}
	}
	if err != nil {
		status := DeliveryError
		if isStaleError(err) {
			status = DeliveryStale
		} else if isOfflineError(err) {
			status = DeliveryOffline
		}
		_ = SaveDeliveryReport(stored.ID, status, err.Error(), "", false)
		return
	}
	if err := SaveDeliveryReport(stored.ID, DeliveryRemoved, "", "", false); err != nil {
		logger.Warningf("clash: delete delivery %s: %v", key, err)
	}
}

func (rt *Runtime) pollDelivery(ctx context.Context, cat *Catalog, p DesiredPolicy, d Delivery) {
	var (
		report PolicyReport
		err    error
	)
	if p.NodeKey == LocalNodeKey {
		report, err = rt.Report(cat.PanelGuid, d.ClientID)
	} else {
		node, lerr := loadNodeForDelivery(p.NodeKey)
		if lerr != nil {
			err = lerr
		} else {
			callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			report, err = reportNode(callCtx, node, cat.PanelGuid, d.ClientID)
		}
	}
	if err != nil {
		status := DeliveryError
		if isOfflineError(err) {
			status = DeliveryOffline
		}
		_ = SaveDeliveryReport(d.ID, status, err.Error(), d.LastReportRaw, d.ResetPending)
		return
	}
	saveAppliedReport(d.ID, report, d.ResetPending)
}

func saveAppliedReport(deliveryID int, report PolicyReport, resetPending bool) {
	b, _ := json.Marshal(report)
	status := DeliveryApplied
	errText := ""
	if !report.Applied || report.Removed {
		if report.Removed {
			status = DeliveryRemoved
		} else {
			status = DeliveryError
			errText = report.ApplyError
			if errText == "" {
				errText = "policy is not applied"
			}
		}
	}
	_ = SaveDeliveryReport(deliveryID, status, errText, string(b), resetPending)
}

func upsertDesiredDelivery(p DesiredPolicy) (*Delivery, error) {
	row := Delivery{
		NodeKey:      p.NodeKey,
		ClientID:     p.ClientID,
		ClientEmail:  p.ClientEmail,
		TargetGuid:   p.NodeGuid,
		Revision:     p.Revision,
		PayloadHash:  p.PayloadHash,
		ResetPending: p.Reset,
		Status:       p.Status,
		LastError:    p.LastError,
		Remove:       p.Remove,
	}
	if err := UpsertDelivery(row); err != nil {
		return nil, err
	}
	return GetDelivery(p.NodeKey, p.ClientID)
}

func isOfflineError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "no route to host") ||
		strings.Contains(msg, "connection reset")
}
