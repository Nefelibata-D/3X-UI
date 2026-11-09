// Package gateway implements the local SOCKS5 data-plane gateway used by the
// Clash quota policy. It is intentionally independent of the panel services:
// the caller supplies configuration, persistence and state-change callbacks.
package gateway

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type State string

const (
	StateNormal   State = "normal"
	StateGrace    State = "grace"
	StateDisabled State = "disabled"
)

const MaxGraceSeconds = int64(600)

// Config is the enforced budget. Grace settings are frozen when grace starts.
type Config struct {
	ResetSchedule      ResetSchedule
	AllocationRequired bool
	BudgetBytes        int64
	GraceSeconds       int64
	GracePercent       int64
	PeriodTimezone     string
}

// PersistedState is the complete ledger for one policy. Usage counters are
// cumulative for the current period and are never reset by a process restart.
type PersistedState struct {
	ResetSchedule   ResetSchedule `json:"resetSchedule"`
	PeriodTimezone  string        `json:"periodTimezone"`
	ResetRevision   int64         `json:"resetRevision"`
	State           State         `json:"state"`
	UsageUp         int64         `json:"usageUp"`
	UsageDown       int64         `json:"usageDown"`
	PeriodStart     int64         `json:"periodStart"`
	PeriodEnd       int64         `json:"periodEnd"`
	GraceStartedAt  int64         `json:"graceStartedAt"`
	GraceDeadline   int64         `json:"graceDeadline"`
	GraceExtraBytes int64         `json:"graceExtraBytes"`
	GraceBaseline   int64         `json:"graceBaseline"`
	DisabledAt      int64         `json:"disabledAt"`
}

type Store interface {
	Save(PersistedState) error
}

type StateChange struct {
	Old State
	New State
	At  time.Time
}

type Callbacks struct {
	OnStateChange func(StateChange)
}

type Report struct {
	State             State
	UsageUp           int64
	UsageDown         int64
	BudgetBytes       int64
	GraceExtraBytes   int64
	PeriodStart       int64
	PeriodEnd         int64
	GraceStartedAt    int64
	GraceDeadline     int64
	GraceBaseline     int64
	ActiveConnections int
	DisabledAt        int64
	LastError         string
}

type activeConn struct {
	abort func()
}

type Policy struct {
	key   string
	store Store
	cb    Callbacks
	now   func() time.Time

	mu        sync.Mutex
	cfg       Config
	loc       *time.Location
	state     State
	usageUp   int64
	usageDown int64

	periodStart    int64
	periodEnd      int64
	graceStartedAt int64
	graceDeadline  int64
	graceExtra     int64
	graceBaseline  int64
	disabledAt     int64
	active         map[int64]*activeConn
	nextID         int64
	lastError      string
	dirty          bool
	resetRevision  int64
	closed         bool
}

func NewPolicy(key string, cfg Config, st PersistedState, store Store, cb Callbacks, now func() time.Time) (*Policy, error) {
	if store == nil {
		return nil, errors.New("gateway: store is required")
	}
	if now == nil {
		now = time.Now
	}
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	loc, err := loadLocation(cfg.PeriodTimezone)
	if err != nil {
		return nil, err
	}
	p := &Policy{
		resetRevision:  st.ResetRevision,
		key:            key,
		store:          store,
		cb:             cb,
		now:            now,
		cfg:            cfg,
		loc:            loc,
		state:          st.State,
		periodStart:    st.PeriodStart,
		periodEnd:      st.PeriodEnd,
		graceStartedAt: st.GraceStartedAt,
		graceDeadline:  st.GraceDeadline,
		graceExtra:     st.GraceExtraBytes,
		graceBaseline:  st.GraceBaseline,
		disabledAt:     st.DisabledAt,
		active:         make(map[int64]*activeConn),
	}
	if p.state == "" {
		p.state = StateNormal
	}
	p.usageUp = st.UsageUp
	p.usageDown = st.UsageDown
	at := now()
	p.mu.Lock()
	if st.PeriodStart != 0 && !sameSchedule(st.ResetSchedule, cfg.ResetSchedule, st.PeriodTimezone, cfg.PeriodTimezone) {
		p.periodStart, p.periodEnd = cfg.ResetSchedule.Bounds(at, loc)
	}
	p.settleLocked(at, true)
	if p.periodStart == 0 {
		p.periodStart, p.periodEnd = p.cfg.ResetSchedule.Bounds(at, p.loc)
	}
	if p.periodStart != st.PeriodStart || p.periodEnd != st.PeriodEnd || p.state != st.State {
		if err := p.persistLocked(); err != nil {
			p.mu.Unlock()
			return nil, err
		}
	}
	p.mu.Unlock()
	return p, nil
}

func validateConfig(cfg Config) error {
	if err := cfg.ResetSchedule.Validate(); err != nil {
		return err
	}
	if cfg.BudgetBytes < 0 {
		return fmt.Errorf("gateway: budget must not be negative, got %d", cfg.BudgetBytes)
	}
	if cfg.GraceSeconds <= 0 || cfg.GraceSeconds > MaxGraceSeconds {
		return fmt.Errorf("gateway: grace seconds must be 1..%d, got %d", MaxGraceSeconds, cfg.GraceSeconds)
	}
	if cfg.GracePercent < 0 || cfg.GracePercent > 100 {
		return fmt.Errorf("gateway: grace percent must be 0..100, got %d", cfg.GracePercent)
	}
	return nil
}

// ValidateConfig validates the enforced quota and billing schedule.
func ValidateConfig(cfg Config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	if _, err := loadLocation(cfg.PeriodTimezone); err != nil {
		return err
	}
	return nil
}

func loadLocation(name string) (*time.Location, error) {
	if name == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("gateway: invalid period timezone %q: %w", name, err)
	}
	return loc, nil
}

func (p *Policy) usageTotalLocked() int64 {
	return p.usageUp + p.usageDown
}

func (p *Policy) admitLocked(at time.Time) (bool, *StateChange, []func()) {
	change, aborts := p.settleLocked(at, false)
	if p.lastError != "" {
		if err := p.persistLocked(); err != nil {
			return false, change, aborts
		}
	}
	// Quota observations do not restrict client routing; the server enforces final cutoff.
	switch p.state {
	case StateDisabled:
		return false, change, aborts
	default:
		return true, change, aborts
	}
}

func (p *Policy) Admit() bool {
	at := p.now()
	p.mu.Lock()
	allowed, change, aborts := p.admitLocked(at)
	p.mu.Unlock()
	p.afterUnlock(change, aborts)
	return allowed
}

func (p *Policy) Track(abort func()) (int64, bool) {
	if abort == nil {
		abort = func() {}
	}
	at := p.now()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, false
	}
	allowed, change, aborts := p.admitLocked(at)
	if !allowed {
		p.mu.Unlock()
		p.afterUnlock(change, aborts)
		return 0, false
	}
	p.nextID++
	id := p.nextID
	p.active[id] = &activeConn{abort: abort}
	p.mu.Unlock()
	p.afterUnlock(change, aborts)
	return id, true
}

func (p *Policy) Untrack(id int64) {
	p.mu.Lock()
	delete(p.active, id)
	p.mu.Unlock()
}

// Account updates the per-user ledger shared by all connections.
func (p *Policy) Account(up, down int64) {
	at := p.now()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	change, aborts := p.settleLocked(at, false)
	before := p.usageUp + p.usageDown
	if up > 0 {
		p.usageUp += up
	}
	if down > 0 {
		p.usageDown += down
	}
	if up > 0 || down > 0 {
		p.dirty = true
	}
	after := p.usageUp + p.usageDown
	var change2 *StateChange
	var aborts2 []func()
	if before != after {
		change2, aborts2 = p.settleLocked(at, false)
	}
	p.mu.Unlock()
	p.afterUnlock(change, aborts)
	p.afterUnlock(change2, aborts2)
}

func (p *Policy) Tick() {
	at := p.now()
	p.mu.Lock()
	change, aborts := p.settleLocked(at, false)
	if p.dirty {
		_ = p.persistLocked()
	}
	p.mu.Unlock()
	p.afterUnlock(change, aborts)
}

func (p *Policy) Report() Report {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Report{
		State:             p.state,
		UsageUp:           p.usageUp,
		UsageDown:         p.usageDown,
		BudgetBytes:       p.cfg.BudgetBytes,
		GraceExtraBytes:   p.graceExtra,
		PeriodStart:       p.periodStart,
		PeriodEnd:         p.periodEnd,
		GraceStartedAt:    p.graceStartedAt,
		GraceDeadline:     p.graceDeadline,
		GraceBaseline:     p.graceBaseline,
		ActiveConnections: len(p.active),
		DisabledAt:        p.disabledAt,
		LastError:         p.lastError,
	}
}

func (p *Policy) ApplyConfig(cfg Config) error {
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	loc, err := loadLocation(cfg.PeriodTimezone)
	if err != nil {
		return err
	}
	at := p.now()
	p.mu.Lock()
	oldState := p.state
	if p.cfg.ResetSchedule != cfg.ResetSchedule || p.cfg.PeriodTimezone != cfg.PeriodTimezone {
		p.periodStart, p.periodEnd = cfg.ResetSchedule.Bounds(at, loc)
		p.dirty = true
	}
	if p.cfg.AllocationRequired && p.cfg.BudgetBytes == 0 && (!cfg.AllocationRequired || cfg.BudgetBytes > 0) {
		p.state = StateNormal
		p.disabledAt = 0
		p.dirty = true
	}
	p.cfg = cfg
	p.loc = loc
	change, aborts := p.settleLocked(at, false)
	if change == nil {
		change = p.changeLocked(oldState, p.state, at)
	}
	if p.dirty {
		if err := p.persistLocked(); err != nil {
			p.mu.Unlock()
			p.afterUnlock(change, aborts)
			return err
		}
	}
	p.mu.Unlock()
	p.afterUnlock(change, aborts)
	return nil
}

// Reset persists the zero ledger before changing memory, so a failed save
// cannot report success or leave restart behavior inconsistent.
func (p *Policy) Reset() error { return p.ResetRevision(0) }

func (p *Policy) ResetRevision(revision int64) error {
	at := p.now()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("gateway: policy is closed")
	}
	if revision > 0 && revision <= p.resetRevision {
		p.mu.Unlock()
		return nil
	}
	if revision == 0 {
		revision = p.resetRevision
	}
	old := p.state
	start, end := p.cfg.ResetSchedule.Bounds(at, p.loc)
	candidate := PersistedState{
		ResetSchedule: p.cfg.ResetSchedule, PeriodTimezone: p.cfg.PeriodTimezone,
		ResetRevision: revision,
		State:         StateNormal,
		PeriodStart:   start,
		PeriodEnd:     end,
	}
	if p.cfg.AllocationRequired && p.cfg.BudgetBytes == 0 {
		candidate.State = StateDisabled
		candidate.DisabledAt = at.Unix()
	}
	if err := p.store.Save(candidate); err != nil {
		p.lastError = err.Error()
		p.mu.Unlock()
		return err
	}
	p.resetRevision = revision
	p.state = candidate.State
	p.usageUp = 0
	p.usageDown = 0
	p.periodStart = start
	p.periodEnd = end
	p.graceStartedAt = 0
	p.graceDeadline = 0
	p.graceExtra = 0
	p.graceBaseline = 0
	p.disabledAt = candidate.DisabledAt
	p.lastError = ""
	p.dirty = false
	change := p.changeLocked(old, candidate.State, at)
	p.mu.Unlock()
	p.afterUnlock(change, nil)
	return nil
}

func (p *Policy) AbortConnections() {
	p.mu.Lock()
	aborts := p.abortAllLocked()
	p.mu.Unlock()
	for _, abort := range aborts {
		abort()
	}
}

func (p *Policy) Close() error {
	p.mu.Lock()
	if p.closed {
		err := p.persistLocked()
		p.mu.Unlock()
		return err
	}
	p.closed = true
	err := p.persistLocked()
	aborts := p.abortAllLocked()
	p.mu.Unlock()
	for _, abort := range aborts {
		abort()
	}
	return err
}

func (p *Policy) settleLocked(at time.Time, startup bool) (*StateChange, []func()) {
	if p.cfg.AllocationRequired && p.cfg.BudgetBytes == 0 {
		if p.state != StateDisabled {
			return p.enterDisabledLocked(at), p.abortAllLocked()
		}
		return nil, nil
	}
	if p.periodEnd > 0 && at.Unix() >= p.periodEnd {
		old := p.state
		p.resetLocked(at)
		return p.changeLocked(old, StateNormal, at), nil
	}
	if p.cfg.BudgetBytes == 0 && p.state != StateNormal {
		old := p.state
		p.state = StateNormal
		p.graceStartedAt, p.graceDeadline, p.graceExtra, p.graceBaseline, p.disabledAt = 0, 0, 0, 0, 0
		_ = p.persistLocked()
		return p.changeLocked(old, StateNormal, at), nil
	}
	switch p.state {
	case StateGrace:
		if at.Unix() >= p.graceDeadline || p.usageTotalLocked() >= p.graceBaseline+p.graceExtra {
			return p.enterDisabledLocked(at), p.abortAllLocked()
		}
	case StateNormal:
		if p.cfg.BudgetBytes > 0 && p.usageTotalLocked() >= p.cfg.BudgetBytes {
			if startup {
				return p.enterDisabledLocked(at), nil
			}
			change, aborts := p.enterGraceLocked(at)
			if p.graceExtra == 0 {
				return p.enterDisabledLocked(at), p.abortAllLocked()
			}
			return change, aborts
		}
	}
	return nil, nil
}

func (p *Policy) enterGraceLocked(at time.Time) (*StateChange, []func()) {
	old := p.state
	p.state = StateGrace
	p.graceStartedAt = at.Unix()
	p.graceDeadline = at.Add(time.Duration(p.cfg.GraceSeconds) * time.Second).Unix()
	p.graceExtra = graceExtraBytes(p.cfg.BudgetBytes, p.cfg.GracePercent)
	p.graceBaseline = p.usageTotalLocked()
	p.disabledAt = 0
	_ = p.persistLocked()
	return p.changeLocked(old, StateGrace, at), nil
}

func (p *Policy) enterDisabledLocked(at time.Time) *StateChange {
	old := p.state
	p.state = StateDisabled
	p.disabledAt = at.Unix()
	_ = p.persistLocked()
	return p.changeLocked(old, StateDisabled, at)
}

func (p *Policy) resetLocked(at time.Time) {
	p.state = StateNormal
	p.usageUp = 0
	p.usageDown = 0
	p.periodStart, p.periodEnd = p.cfg.ResetSchedule.Bounds(at, p.loc)
	p.graceStartedAt = 0
	p.graceDeadline = 0
	p.graceExtra = 0
	p.graceBaseline = 0
	p.disabledAt = 0
	p.dirty = true
}

func (p *Policy) changeLocked(old, next State, at time.Time) *StateChange {
	if old == next {
		return nil
	}
	return &StateChange{Old: old, New: next, At: at}
}

func (p *Policy) abortAllLocked() []func() {
	aborts := make([]func(), 0, len(p.active))
	for id, conn := range p.active {
		aborts = append(aborts, conn.abort)
		delete(p.active, id)
	}
	return aborts
}

func (p *Policy) stateLocked() PersistedState {
	st := PersistedState{
		ResetSchedule: p.cfg.ResetSchedule, PeriodTimezone: p.cfg.PeriodTimezone,
		ResetRevision:   p.resetRevision,
		State:           p.state,
		UsageUp:         p.usageUp,
		UsageDown:       p.usageDown,
		PeriodStart:     p.periodStart,
		PeriodEnd:       p.periodEnd,
		GraceStartedAt:  p.graceStartedAt,
		GraceDeadline:   p.graceDeadline,
		GraceExtraBytes: p.graceExtra,
		GraceBaseline:   p.graceBaseline,
		DisabledAt:      p.disabledAt,
	}
	return st
}

func (p *Policy) persistLocked() error {
	if err := p.store.Save(p.stateLocked()); err != nil {
		p.dirty = true
		p.lastError = err.Error()
		return err
	}
	p.lastError = ""
	p.dirty = false
	return nil
}

func (p *Policy) afterUnlock(change *StateChange, aborts []func()) {
	for _, abort := range aborts {
		abort()
	}
	if change != nil && p.cb.OnStateChange != nil {
		p.cb.OnStateChange(*change)
	}
}

func graceExtraBytes(budget, percent int64) int64 {
	if budget <= 0 || percent <= 0 {
		return 0
	}
	return (budget/100)*percent + ((budget%100)*percent+99)/100
}
