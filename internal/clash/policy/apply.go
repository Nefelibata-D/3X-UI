package policy

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
	"gorm.io/gorm"
)

var (
	ErrStaleRevision  = errors.New("stale policy revision")
	ErrTargetMismatch = errors.New("policy target does not match this panel")
	ErrPolicyNotFound = errors.New("policy not found")
	ErrPolicyRemoving = errors.New("policy removal is in progress")
)

type ApplyRequest struct {
	ResetSchedule      gateway.ResetSchedule `json:"resetSchedule"`
	AllocationRequired bool                  `json:"allocationRequired"`
	MasterGuid         string                `json:"masterGuid"`
	TargetGuid         string                `json:"targetGuid"`
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
}

func (r ApplyRequest) Validate() error {
	if strings.TrimSpace(r.MasterGuid) == "" {
		return errors.New("masterGuid is required")
	}
	if strings.TrimSpace(r.TargetGuid) == "" {
		return errors.New("targetGuid is required")
	}
	if r.ClientID <= 0 {
		return errors.New("clientId must be positive")
	}
	if strings.TrimSpace(r.ClientEmail) == "" {
		return errors.New("clientEmail is required")
	}
	if r.Revision <= 0 {
		return errors.New("revision must be positive")
	}
	if r.Remove {
		return nil
	}
	if r.EffectiveRatio < 0 || r.EffectiveRatio > 1.0000001 || math.IsNaN(r.EffectiveRatio) {
		return errors.New("effectiveRatio must be 0..1")
	}
	return gateway.ValidateConfig(r.config())
}

func (r ApplyRequest) config() gateway.Config {
	return gateway.Config{
		BudgetBytes:   r.BudgetBytes,
		GraceSeconds:  r.GraceSeconds,
		GracePercent:  r.GracePercent,
		ResetSchedule: r.ResetSchedule, AllocationRequired: r.AllocationRequired,
		PeriodTimezone: r.PeriodTimezone,
	}
}

type PolicyReport struct {
	MasterGuid        string           `json:"masterGuid"`
	ClientID          int              `json:"clientId"`
	ClientEmail       string           `json:"clientEmail"`
	Revision          int64            `json:"revision"`
	State             string           `json:"state"`
	BudgetBytes       int64            `json:"budgetBytes"`
	GraceSeconds      int64            `json:"graceSeconds"`
	GracePercent      int64            `json:"gracePercent"`
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
	AppliedAt         int64            `json:"appliedAt"`
	DisabledAt        int64            `json:"disabledAt"`
	LastError         string           `json:"lastError,omitempty"`
	Removing          bool             `json:"removing,omitempty"`
	Removed           bool             `json:"removed,omitempty"`
}

func (pr *PolicyRuntime) report() PolicyReport {
	pr.mu.Lock()
	managed := append([]ManagedInbound(nil), pr.managed...)
	cfg := struct {
		masterGuid string
		clientID   int
		email      string
		revision   int64
		ratio      float64
		budget     int64
		graceSecs  int64
		gracePct   int64
		port       int
		listening  bool
		applied    bool
		applyError string
		removing   bool
	}{
		masterGuid: pr.masterGuid,
		clientID:   pr.clientID,
		email:      pr.clientEmail,
		revision:   pr.revision,
		ratio:      pr.ratio,
		budget:     pr.budget,
		graceSecs:  pr.graceSecs,
		gracePct:   pr.gracePct,
		port:       pr.port,
		listening:  pr.server != nil,
		applied:    pr.applied,
		applyError: pr.applyError,
		removing:   pr.removing,
	}
	pr.mu.Unlock()

	out := PolicyReport{
		MasterGuid:       cfg.masterGuid,
		ClientID:         cfg.clientID,
		ClientEmail:      cfg.email,
		Revision:         cfg.revision,
		BudgetBytes:      cfg.budget,
		GraceSeconds:     cfg.graceSecs,
		GracePercent:     cfg.gracePct,
		EffectiveRatio:   cfg.ratio,
		GatewayPort:      cfg.port,
		GatewayListening: cfg.listening,
		ManagedInbounds:  managed,
		Applied:          cfg.applied,
		ApplyError:       cfg.applyError,
		Removing:         cfg.removing,
	}
	if pr.policy != nil {
		r := pr.policy.Report()
		out.State = string(r.State)
		out.UsageUp = r.UsageUp
		out.UsageDown = r.UsageDown
		out.PeriodStart = r.PeriodStart
		out.PeriodEnd = r.PeriodEnd
		out.GraceStartedAt = r.GraceStartedAt
		out.GraceDeadline = r.GraceDeadline
		out.GraceExtraBytes = r.GraceExtraBytes
		out.ActiveConnections = r.ActiveConnections
		out.DisabledAt = r.DisabledAt
		out.LastError = r.LastError
	}
	return out
}

func reportFromRow(row AppliedPolicy) PolicyReport {
	out := PolicyReport{
		MasterGuid:      row.MasterGuid,
		ClientID:        row.ClientID,
		ClientEmail:     row.ClientEmail,
		Revision:        row.Revision,
		State:           row.State,
		BudgetBytes:     row.BudgetBytes,
		GraceSeconds:    row.GraceSeconds,
		GracePercent:    row.GracePercent,
		EffectiveRatio:  row.EffectiveRatio,
		UsageUp:         row.UsageUp,
		UsageDown:       row.UsageDown,
		PeriodStart:     row.PeriodStart,
		PeriodEnd:       row.PeriodEnd,
		GraceStartedAt:  row.GraceStartedAt,
		GraceDeadline:   row.GraceDeadline,
		GraceExtraBytes: row.GraceExtraBytes,
		GatewayPort:     row.GatewayPort,
		ManagedInbounds: row.ManagedInbounds(),
		Applied:         false,
		ApplyError:      row.ApplyError,
		DisabledAt:      row.DisabledAt,
		Removing:        row.Removing,
		Removed:         row.Removed,
	}
	if row.Removed {
		out.State = "removed"
		out.GatewayPort = 0
		out.GatewayListening = false
	}
	return out
}

func (rt *Runtime) Report(masterGuid string, clientID int) (PolicyReport, error) {
	if pr := rt.findPolicy(masterGuid, clientID); pr != nil {
		return pr.report(), nil
	}
	row, err := GetAppliedPolicy(masterGuid, clientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return PolicyReport{}, ErrPolicyNotFound
		}
		return PolicyReport{}, err
	}
	return reportFromRow(*row), nil
}

// Apply enforces one policy on this panel. Local master policies and remote
// child policies share this path; the target GUID always binds the request to
// this panel's stable identity.
func (rt *Runtime) Apply(req ApplyRequest) (PolicyReport, error) {
	if err := req.Validate(); err != nil {
		return PolicyReport{}, err
	}
	cat, err := rt.catalogNow()
	if err != nil {
		return PolicyReport{}, err
	}
	if req.TargetGuid != cat.PanelGuid {
		return PolicyReport{}, ErrTargetMismatch
	}

	unlock := rt.lockPolicy(policyKey(req.MasterGuid, req.ClientID))
	defer unlock()

	existing, err := GetAppliedPolicy(req.MasterGuid, req.ClientID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return PolicyReport{}, err
	}
	if req.Remove {
		return rt.removeLocked(existing, req)
	}
	if _, ok := cat.ClientByEmail(req.ClientEmail); !ok {
		return PolicyReport{}, catalog.ErrClientNotFound
	}
	// Remote ownership supersedes this panel's local policy for the same user.
	rows, err := ListAppliedPolicies()
	if err != nil {
		return PolicyReport{}, err
	}
	for _, row := range rows {
		if row.Removed || row.ClientEmail != req.ClientEmail || row.MasterGuid == cat.PanelGuid {
			continue
		}
		if req.MasterGuid == cat.PanelGuid {
			return rt.Report(row.MasterGuid, row.ClientID)
		}
		if row.MasterGuid != req.MasterGuid || row.ClientID != req.ClientID {
			return PolicyReport{}, &ConflictError{Tag: firstManagedTag(row.ManagedInbounds()), Owner: policyKey(row.MasterGuid, row.ClientID)}
		}
	}
	if existing != nil {
		switch {
		case existing.Removed:
			if req.Revision <= existing.Revision {
				return reportFromRow(*existing), ErrStaleRevision
			}
		case existing.Removing:
			if req.Revision <= existing.Revision {
				return reportFromRow(*existing), ErrStaleRevision
			}
			return reportFromRow(*existing), ErrPolicyRemoving
		case existing.Revision > req.Revision:
			return reportFromRow(*existing), ErrStaleRevision
		case existing.Revision == req.Revision:
			if !samePolicy(*existing, req) {
				return reportFromRow(*existing), ErrStaleRevision
			}
			return rt.updatePolicyLocked(cat, *existing, req, existing.ManagedInbounds())
		}
	}

	if req.MasterGuid != cat.PanelGuid {
		for _, row := range rows {
			if row.Removed || row.MasterGuid != cat.PanelGuid || row.ClientEmail != req.ClientEmail {
				continue
			}
			if err := rt.removeExistingLocked(&row, row.Revision); err != nil {
				return PolicyReport{}, err
			}
		}
	}
	managed := resolveManaged(cat, req.ClientEmail)
	if err := rt.tagConflict(req.MasterGuid, req.ClientID, req.ClientEmail, managedTags(managed)); err != nil {
		return PolicyReport{}, err
	}
	if existing == nil {
		return rt.createPolicyLocked(cat, req, managed)
	}
	return rt.updatePolicyLocked(cat, *existing, req, managed)
}

func managedTags(managed []ManagedInbound) []string {
	out := make([]string, 0, len(managed))
	for _, m := range managed {
		if m.Tag != "" {
			out = append(out, m.Tag)
		}
	}
	return out
}

func (rt *Runtime) createPolicyLocked(cat *catalog.Catalog, req ApplyRequest, managed []ManagedInbound) (PolicyReport, error) {
	row := &AppliedPolicy{
		MasterGuid:     req.MasterGuid,
		ClientID:       req.ClientID,
		ClientEmail:    req.ClientEmail,
		Revision:       req.Revision,
		BudgetBytes:    req.BudgetBytes,
		GraceSeconds:   req.GraceSeconds,
		GracePercent:   req.GracePercent,
		EffectiveRatio: req.EffectiveRatio,
		ResetSchedule:  req.ResetSchedule, AllocationRequired: req.AllocationRequired,
		PeriodTimezone: req.PeriodTimezone,
		State:          string(gateway.StateNormal),
	}
	if err := CreateAppliedPolicy(row); err != nil {
		return PolicyReport{}, err
	}
	pr := &PolicyRuntime{
		key:         policyKey(req.MasterGuid, req.ClientID),
		rowID:       row.ID,
		masterGuid:  req.MasterGuid,
		clientID:    req.ClientID,
		clientEmail: req.ClientEmail,
		revision:    req.Revision,
		ratio:       req.EffectiveRatio,
		budget:      req.BudgetBytes,
		graceSecs:   req.GraceSeconds,
		gracePct:    req.GracePercent,
		timezone:    req.PeriodTimezone,
		managed:     managed,
		needsApply:  true,
	}
	cb := gateway.Callbacks{OnStateChange: func(change gateway.StateChange) {
		rt.enqueue(pr, change)
	}}
	policy, err := gateway.NewPolicy(pr.key, req.config(), gateway.PersistedState{}, ledgerStore{rowID: row.ID}, cb, time.Now)
	if err != nil {
		_ = UpdateAppliedInbounds(row.ID, managed, false, err.Error())
		return PolicyReport{}, err
	}
	pr.policy = policy
	rt.mu.Lock()
	rt.policies[pr.key] = pr
	rt.mu.Unlock()
	if req.Reset {
		if err := policy.ResetRevision(req.Revision); err != nil {
			_ = rt.persistApplyError(pr, err)
			return pr.report(), err
		}
	}
	if err := rt.startServer(pr); err != nil {
		rt.reconcilePolicyLocked(pr, cat, true)
		return pr.report(), err
	}
	rt.reconcilePolicyLocked(pr, cat, true)
	return pr.report(), nil
}

func (rt *Runtime) updatePolicyLocked(cat *catalog.Catalog, row AppliedPolicy, req ApplyRequest, managed []ManagedInbound) (PolicyReport, error) {
	if err := gateway.ValidateConfig(req.config()); err != nil {
		return PolicyReport{}, err
	}
	pr := rt.findPolicy(req.MasterGuid, req.ClientID)
	next := row
	next.ClientEmail = req.ClientEmail
	next.Revision = req.Revision
	next.BudgetBytes = req.BudgetBytes
	next.GraceSeconds = req.GraceSeconds
	next.GracePercent = req.GracePercent
	next.EffectiveRatio = req.EffectiveRatio
	next.PeriodTimezone = req.PeriodTimezone
	next.ResetSchedule = req.ResetSchedule
	next.AllocationRequired = req.AllocationRequired
	next.Removing = false
	next.Removed = false
	if next.State == "removed" {
		next.State = string(gateway.StateNormal)
	}
	if pr != nil {
		pr.mu.Lock()
		next.GatewayPort = pr.port
		pr.mu.Unlock()
	}
	if err := UpdateAppliedConfig(row.ID, next); err != nil {
		return PolicyReport{}, err
	}
	row = next

	if pr == nil || pr.policy == nil {
		if err := rt.loadPolicy(row); err != nil {
			return PolicyReport{}, err
		}
		pr = rt.findPolicy(req.MasterGuid, req.ClientID)
		if pr == nil || pr.policy == nil {
			return PolicyReport{}, errors.New("failed to load persisted policy")
		}
	} else {
		pr.mu.Lock()
		// The routing identity must refresh even when the inbound tags stay unchanged.
		if pr.clientEmail != req.ClientEmail {
			pr.needsApply = true
		}
		pr.clientEmail = req.ClientEmail
		pr.revision = req.Revision
		pr.ratio = req.EffectiveRatio
		pr.budget = req.BudgetBytes
		pr.graceSecs = req.GraceSeconds
		pr.gracePct = req.GracePercent
		pr.timezone = req.PeriodTimezone
		pr.removing = false
		pr.mu.Unlock()
		if err := pr.policy.ApplyConfig(req.config()); err != nil {
			return pr.report(), err
		}
	}
	if pr.server == nil {
		if err := rt.startServer(pr); err != nil {
			rt.reconcilePolicyLocked(pr, cat, true)
			return pr.report(), err
		}
	}
	if req.Reset && row.ResetAppliedRevision < req.Revision {
		if err := pr.policy.ResetRevision(req.Revision); err != nil {
			_ = rt.persistApplyError(pr, err)
			return pr.report(), err
		}
	}
	rt.reconcilePolicyLocked(pr, cat, true)
	return pr.report(), nil
}

func (rt *Runtime) persistApplyError(pr *PolicyRuntime, err error) error {
	pr.mu.Lock()
	pr.applyError = err.Error()
	pr.applied = false
	managed := append([]ManagedInbound(nil), pr.managed...)
	pr.mu.Unlock()
	return UpdateAppliedInbounds(pr.rowID, managed, false, err.Error())
}

// Remove is the explicit removal entry point. Apply with Remove also uses the
// same serialized path.
func (rt *Runtime) Remove(req ApplyRequest) (PolicyReport, error) {
	if err := req.Validate(); err != nil {
		return PolicyReport{}, err
	}
	cat, err := rt.catalogNow()
	if err != nil {
		return PolicyReport{}, err
	}
	if req.TargetGuid != cat.PanelGuid {
		return PolicyReport{}, ErrTargetMismatch
	}
	unlock := rt.lockPolicy(policyKey(req.MasterGuid, req.ClientID))
	defer unlock()
	existing, err := GetAppliedPolicy(req.MasterGuid, req.ClientID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return rt.createTombstoneLocked(req)
		}
		return PolicyReport{}, err
	}
	return rt.removeLocked(existing, req)
}

func (rt *Runtime) createTombstoneLocked(req ApplyRequest) (PolicyReport, error) {
	row := &AppliedPolicy{
		MasterGuid:  req.MasterGuid,
		ClientID:    req.ClientID,
		ClientEmail: req.ClientEmail,
		Revision:    req.Revision,
		State:       string(gateway.StateNormal),
		Removed:     true,
	}
	if err := CreateAppliedPolicy(row); err != nil {
		return PolicyReport{}, err
	}
	return reportFromRow(*row), nil
}

func (rt *Runtime) removeLocked(existing *AppliedPolicy, req ApplyRequest) (PolicyReport, error) {
	if existing == nil {
		return rt.createTombstoneLocked(req)
	}
	if existing.Removed {
		if req.Revision < existing.Revision {
			return reportFromRow(*existing), ErrStaleRevision
		}
		if req.Revision > existing.Revision {
			if err := FinishAppliedRemoval(existing.ID, req.Revision, existing.ManagedInbounds()); err != nil {
				return reportFromRow(*existing), err
			}
			existing.Revision = req.Revision
		}
		return reportFromRow(*existing), nil
	}
	if existing.Removing {
		if req.Revision < existing.Revision {
			return reportFromRow(*existing), ErrStaleRevision
		}
	} else if existing.Revision > req.Revision {
		return reportFromRow(*existing), ErrStaleRevision
	}
	if err := rt.removeExistingLocked(existing, req.Revision); err != nil {
		return PolicyReport{}, err
	}
	row, err := GetAppliedPolicy(req.MasterGuid, req.ClientID)
	if err != nil {
		return PolicyReport{}, err
	}
	return reportFromRow(*row), nil
}

func (rt *Runtime) removeExistingLocked(existing *AppliedPolicy, revision int64) error {
	pr := rt.findPolicy(existing.MasterGuid, existing.ClientID)
	managed := existing.ManagedInbounds()
	if pr != nil {
		pr.mu.Lock()
		managed = append([]ManagedInbound(nil), pr.managed...)
		pr.removing = true
		pr.mu.Unlock()
	}
	if !existing.Removing || revision > existing.Revision {
		if err := MarkAppliedRemoving(existing.ID, revision); err != nil {
			if pr != nil {
				pr.mu.Lock()
				pr.removing = false
				pr.mu.Unlock()
			}
			return err
		}
		existing.Removing = true
		existing.Removed = false
		existing.Revision = revision
	}
	if pr != nil {
		pr.mu.Lock()
		if pr.server != nil {
			if err := pr.server.Close(); err != nil {
				pr.mu.Unlock()
				return err
			}
			pr.server = nil
		}
		pr.mu.Unlock()
	}

	if rt.hooks.ApplyXray != nil {
		if err := rt.hooks.ApplyXray(); err != nil {
			if pr != nil {
				pr.mu.Lock()
				pr.applyError = "apply xray: " + err.Error()
				pr.applied = false
				pr.mu.Unlock()
			}
			return fmt.Errorf("apply xray: %w", err)
		}
	}
	if err := FinishAppliedRemoval(existing.ID, revision, managed); err != nil {
		if pr != nil {
			pr.mu.Lock()
			pr.applyError = err.Error()
			pr.applied = false
			pr.mu.Unlock()
		}
		return err
	}
	if pr != nil {
		rt.mu.Lock()
		delete(rt.policies, pr.key)
		rt.mu.Unlock()
	}
	return nil
}

func (rt *Runtime) resumeRemovals() {
	rt.mu.RLock()
	policies := make([]*PolicyRuntime, 0)
	for _, pr := range rt.policies {
		pr.mu.Lock()
		removing := pr.removing
		pr.mu.Unlock()
		if removing {
			policies = append(policies, pr)
		}
	}
	rt.mu.RUnlock()
	for _, pr := range policies {
		unlock := rt.lockPolicy(pr.key)
		row, err := GetAppliedPolicy(pr.masterGuid, pr.clientID)
		if err == nil && row.Removing {
			err = rt.removeExistingLocked(row, row.Revision)
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			// The next reconcile/start retries; the row remains removing.
			_ = err
		}
		unlock()
	}
}

func samePolicy(row AppliedPolicy, req ApplyRequest) bool {
	return row.AllocationRequired == req.AllocationRequired && row.ResetSchedule == req.ResetSchedule && row.ClientEmail == req.ClientEmail &&
		row.BudgetBytes == req.BudgetBytes &&
		row.GraceSeconds == req.GraceSeconds &&
		row.GracePercent == req.GracePercent &&
		row.EffectiveRatio == req.EffectiveRatio &&
		row.PeriodTimezone == req.PeriodTimezone
}

func (pr *PolicyRuntime) takeNeedsApply() bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if !pr.needsApply {
		return false
	}
	pr.needsApply = false
	return true
}

func firstManagedTag(managed []ManagedInbound) string {
	if len(managed) == 0 {
		return ""
	}
	return managed[0].Tag
}
