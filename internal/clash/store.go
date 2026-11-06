package clash

import (
	"encoding/json"
	"errors"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/passkey"
	"github.com/mhsanaei/3x-ui/v3/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func Migrate() error {
	db := database.GetDB()
	if db == nil {
		return errors.New("clash: database is not initialized")
	}
	if err := db.AutoMigrate(
		&RuleSet{},
		&YAMLTemplate{},
		&SubscriptionBinding{},
		&NodeWeight{},
		&ClientRatio{},
		&Delivery{},
		&AppliedPolicy{},
	); err != nil {
		return err
	}
	if err := passkey.New(db).Migrate(); err != nil {
		return err
	}
	return migratePublications(db)
}

func dbOrErr() (*gorm.DB, error) {
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("clash: database is not initialized")
	}
	return db, nil
}

func ListNodeWeights() ([]NodeWeight, error) {
	db, err := dbOrErr()
	if err != nil {
		return nil, err
	}
	var rows []NodeWeight
	err = db.Order("node_key ASC").Find(&rows).Error
	return rows, err
}

func UpsertNodeWeights(rows []NodeWeight) error {
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	now := nowMillis()
	return db.Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			row := rows[i]
			if row.CreatedAt == 0 {
				row.CreatedAt = now
			}
			row.UpdatedAt = now
			if err := tx.Model(&NodeWeight{}).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "node_key"}},
				DoUpdates: clause.AssignmentColumns([]string{"node_id", "node_guid", "default_weight", "capacity_bytes", "quota_reset_day", "quota_reset_minute", "quota_reset_disabled", "updated_at"}),
			}).Create(map[string]any{
				"node_key": row.NodeKey, "node_id": row.NodeID, "node_guid": row.NodeGuid,
				"default_weight": row.DefaultWeight,
				"capacity_bytes": row.CapacityBytes, "quota_reset_day": row.ResetSchedule.Day, "quota_reset_minute": row.ResetSchedule.Minute, "quota_reset_disabled": row.ResetSchedule.Disabled,
				"created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ListRatios() ([]ClientRatio, error) {
	db, err := dbOrErr()
	if err != nil {
		return nil, err
	}
	var rows []ClientRatio
	err = db.Order("client_id ASC").Find(&rows).Error
	return rows, err
}

func GetRatio(clientID int) (*ClientRatio, error) {
	db, err := dbOrErr()
	if err != nil {
		return nil, err
	}
	var row ClientRatio
	if err := db.Where("client_id = ?", clientID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func UpsertRatio(row ClientRatio) error {
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	now := nowMillis()
	if row.CreatedAt == 0 {
		row.CreatedAt = now
	}
	row.UpdatedAt = now
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "client_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"email", "mode", "weights_json", "limits_json", "reset_mode", "updated_at"}),
	}).Create(&row).Error
}

func ListDeliveries() ([]Delivery, error) {
	db, err := dbOrErr()
	if err != nil {
		return nil, err
	}
	var rows []Delivery
	err = db.Order("node_key ASC, client_id ASC").Find(&rows).Error
	return rows, err
}

func GetDelivery(nodeKey string, clientID int) (*Delivery, error) {
	db, err := dbOrErr()
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
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	now := nowMillis()
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
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	return db.Model(&Delivery{}).Where("id = ?", id).Updates(map[string]any{
		"status":           status,
		"last_error":       lastError,
		"last_report_json": reportJSON,
		"last_report_at":   nowMillis(),
		"reset_pending":    resetPending,
		"updated_at":       nowMillis(),
	}).Error
}

func ListAppliedPolicies() ([]AppliedPolicy, error) {
	db, err := dbOrErr()
	if err != nil {
		return nil, err
	}
	var rows []AppliedPolicy
	err = db.Order("id ASC").Find(&rows).Error
	return rows, err
}

func GetAppliedPolicy(masterGuid string, clientID int) (*AppliedPolicy, error) {
	db, err := dbOrErr()
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
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	now := nowMillis()
	row.CreatedAt = now
	row.UpdatedAt = now
	return db.Create(row).Error
}

func UpdateAppliedConfig(id int, row AppliedPolicy) error {
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"client_email":         row.ClientEmail,
		"revision":             row.Revision,
		"budget_bytes":         row.BudgetBytes,
		"grace_seconds":        row.GraceSeconds,
		"grace_percent":        row.GracePercent,
		"activity_seconds":     row.ActivitySeconds,
		"site_groups_json":     row.SiteGroupsJSON,
		"effective_ratio":      row.EffectiveRatio,
		"period_timezone":      row.PeriodTimezone,
		"allocation_required":  row.AllocationRequired,
		"quota_reset_day":      row.ResetSchedule.Day,
		"quota_reset_minute":   row.ResetSchedule.Minute,
		"quota_reset_disabled": row.ResetSchedule.Disabled,
		"gateway_port":         row.GatewayPort,
		"removing":             false,
		"removed":              false,
		"updated_at":           nowMillis(),
	}).Error
}

func MarkAppliedRemoving(id int, revision int64) error {
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"removing":    true,
		"removed":     false,
		"revision":    revision,
		"applied":     false,
		"apply_error": "",
		"updated_at":  nowMillis(),
	}).Error
}

func FinishAppliedRemoval(id int, revision int64, managed []ManagedInbound) error {
	db, err := dbOrErr()
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
		"updated_at":   nowMillis(),
	}).Error
}

func UpdateAppliedInbounds(id int, managed []ManagedInbound, applied bool, applyError string) error {
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	b, _ := json.Marshal(managed)
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"managed_json": b,
		"applied":      applied,
		"apply_error":  applyError,
		"updated_at":   nowMillis(),
	}).Error
}

func SaveAppliedLedger(id int, st gateway.PersistedState) error {
	db, err := dbOrErr()
	if err != nil {
		return err
	}
	snapshot, _ := json.Marshal(st.GraceSnapshot)
	return db.Model(&AppliedPolicy{}).Where("id = ?", id).Updates(map[string]any{
		"reset_applied_revision": st.ResetRevision,
		"ledger_reset_day":       st.ResetSchedule.Day, "ledger_reset_minute": st.ResetSchedule.Minute, "ledger_reset_disabled": st.ResetSchedule.Disabled, "ledger_period_timezone": st.PeriodTimezone,
		"grace_groups_json":   EncodeSiteGroups(st.GraceGroups),
		"state":               string(st.State),
		"usage_up":            st.UsageUp,
		"usage_down":          st.UsageDown,
		"period_start":        st.PeriodStart,
		"period_end":          st.PeriodEnd,
		"grace_started_at":    st.GraceStartedAt,
		"grace_deadline":      st.GraceDeadline,
		"grace_extra_bytes":   st.GraceExtraBytes,
		"grace_baseline":      st.GraceBaseline,
		"grace_snapshot_json": string(snapshot),
		"disabled_at":         st.DisabledAt,
		"updated_at":          nowMillis(),
	}).Error
}

func AppliedPolicyByGatewayState(row AppliedPolicy) gateway.PersistedState {
	return gateway.PersistedState{
		ResetSchedule: row.LedgerResetSchedule, PeriodTimezone: row.LedgerPeriodTimezone,
		ResetRevision:   row.ResetAppliedRevision,
		GraceGroups:     row.GraceGroups(),
		State:           gateway.State(row.State),
		UsageUp:         row.UsageUp,
		UsageDown:       row.UsageDown,
		PeriodStart:     row.PeriodStart,
		PeriodEnd:       row.PeriodEnd,
		GraceStartedAt:  row.GraceStartedAt,
		GraceDeadline:   row.GraceDeadline,
		GraceExtraBytes: row.GraceExtraBytes,
		GraceBaseline:   row.GraceBaseline,
		GraceSnapshot:   row.GraceSnapshot(),
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
