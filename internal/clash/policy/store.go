package policy

import (
	"encoding/json"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"gorm.io/gorm/clause"
)

func ListDeliveries() ([]Delivery, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var rows []Delivery
	err = db.Order("node_key ASC, client_id ASC").Find(&rows).Error
	return rows, err
}

func GetDelivery(nodeKey string, clientID int) (*Delivery, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var row Delivery
	if err := db.Where("node_key = ? AND client_id = ?", nodeKey, clientID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func UpsertDelivery(row Delivery) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if row.CreatedAt == 0 {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "node_key"}, {Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"client_email", "target_guid", "revision", "payload_hash", "reset_pending",
			"status", "last_error", "remove", "updated_at",
		}),
	}).Create(&row).Error
}

func SaveDeliveryReport(id int, status, lastError, reportJSON string, resetPending bool) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	return db.Model(&Delivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           status,
		"last_error":       lastError,
		"last_report_json": reportJSON,
		"last_report_at":   time.Now().UnixMilli(),
		"reset_pending":    resetPending,
		"updated_at":       time.Now().UnixMilli(),
	}).Error
}

func ListAppliedPolicies() ([]AppliedPolicy, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var rows []AppliedPolicy
	err = db.Order("id ASC").Find(&rows).Error
	return rows, err
}

func GetAppliedPolicy(masterGuid string, clientID int) (*AppliedPolicy, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var row AppliedPolicy
	if err := db.Where("master_guid = ? AND client_id = ?", masterGuid, clientID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func CreateAppliedPolicy(row *AppliedPolicy) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	row.CreatedAt = now
	row.UpdatedAt = now
	return db.Create(row).Error
}

func UpdateAppliedConfig(id int, row AppliedPolicy) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"client_email":         row.ClientEmail,
		"revision":             row.Revision,
		"budget_bytes":         row.BudgetBytes,
		"grace_seconds":        row.GraceSeconds,
		"grace_percent":        row.GracePercent,
		"effective_ratio":      row.EffectiveRatio,
		"period_timezone":      row.PeriodTimezone,
		"allocation_required":  row.AllocationRequired,
		"quota_reset_day":      row.ResetSchedule.Day,
		"quota_reset_minute":   row.ResetSchedule.Minute,
		"quota_reset_disabled": row.ResetSchedule.Disabled,
		"gateway_port":         row.GatewayPort,
		"removing":             false,
		"removed":              false,
		"updated_at":           time.Now().UnixMilli(),
	}).Error
}

func MarkAppliedRemoving(id int, revision int64) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"removing":    true,
		"removed":     false,
		"revision":    revision,
		"applied":     false,
		"apply_error": "",
		"updated_at":  time.Now().UnixMilli(),
	}).Error
}

func FinishAppliedRemoval(id int, revision int64, managed []ManagedInbound) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(managed)
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"removing":     false,
		"removed":      true,
		"revision":     revision,
		"gateway_port": 0,
		"managed_json": b,
		"applied":      false,
		"apply_error":  "",
		"updated_at":   time.Now().UnixMilli(),
	}).Error
}

func UpdateAppliedInbounds(id int, managed []ManagedInbound, applied bool, applyError string) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(managed)
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"managed_json": b,
		"applied":      applied,
		"apply_error":  applyError,
		"updated_at":   time.Now().UnixMilli(),
	}).Error
}

func SaveAppliedLedger(id int, st gateway.PersistedState) error {
	db, err := storage.DB()
	if err != nil {
		return err
	}
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"reset_applied_revision": st.ResetRevision,
		"ledger_reset_day":       st.ResetSchedule.Day, "ledger_reset_minute": st.ResetSchedule.Minute, "ledger_reset_disabled": st.ResetSchedule.Disabled, "ledger_period_timezone": st.PeriodTimezone,
		"state":             string(st.State),
		"usage_up":          st.UsageUp,
		"usage_down":        st.UsageDown,
		"period_start":      st.PeriodStart,
		"period_end":        st.PeriodEnd,
		"grace_started_at":  st.GraceStartedAt,
		"grace_deadline":    st.GraceDeadline,
		"grace_extra_bytes": st.GraceExtraBytes,
		"grace_baseline":    st.GraceBaseline,
		"disabled_at":       st.DisabledAt,
		"updated_at":        time.Now().UnixMilli(),
	}).Error
}

func AppliedPolicyByGatewayState(row AppliedPolicy) gateway.PersistedState {
	return gateway.PersistedState{
		ResetSchedule: row.LedgerResetSchedule, PeriodTimezone: row.LedgerPeriodTimezone,
		ResetRevision:   row.ResetAppliedRevision,
		State:           gateway.State(row.State),
		UsageUp:         row.UsageUp,
		UsageDown:       row.UsageDown,
		PeriodStart:     row.PeriodStart,
		PeriodEnd:       row.PeriodEnd,
		GraceStartedAt:  row.GraceStartedAt,
		GraceDeadline:   row.GraceDeadline,
		GraceExtraBytes: row.GraceExtraBytes,
		GraceBaseline:   row.GraceBaseline,
		DisabledAt:      row.DisabledAt,
	}
}

func ManagedJSON(in []ManagedInbound) string {
	if len(in) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(in)
	return string(b)
}
