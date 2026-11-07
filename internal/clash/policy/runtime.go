package policy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/catalog"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/gateway"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

type Hooks struct {
	// ApplyXray reconciles the running Xray config after routing/inbound
	// changes. It is the upstream XrayService.RestartXray(false) path.
	ApplyXray func() error
}

type CatalogProvider func() (*catalog.Catalog, error)

type PolicyRuntime struct {
	key         string
	rowID       int
	masterGuid  string
	clientID    int
	clientEmail string
	revision    int64
	ratio       float64
	budget      int64
	graceSecs   int64
	gracePct    int64
	timezone    string

	port   int
	server *gateway.Server
	policy *gateway.Policy

	mu sync.Mutex

	managed    []ManagedInbound
	applied    bool
	applyError string
	needsApply bool
	removing   bool
}

type Runtime struct {
	mu       sync.RWMutex
	policies map[string]*PolicyRuntime

	opMu sync.Mutex

	hooks      Hooks
	catalog    CatalogProvider
	initialize func() error

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wake   chan struct{}
	wg     sync.WaitGroup

	startErr  error
	startOnce sync.Once
	started   bool
	syncMu    sync.Mutex
	statusMu  sync.Mutex

	syncStatus string
	syncError  string
	lastSync   int64
}

var (
	globalMu sync.Mutex
	global   *Runtime
)

func Configure(h Hooks, provider CatalogProvider, initialize func() error) *Runtime {
	globalMu.Lock()
	defer globalMu.Unlock()
	if global == nil {
		global = NewRuntime(h, provider, initialize)
	}
	global.hooks = h
	global.catalog = provider
	global.initialize = initialize
	return global
}

// NewRuntime builds an independent runtime. The panel keeps the package-global
// instance; tests and embedded callers can own a runtime with its own DB.
func NewRuntime(h Hooks, provider CatalogProvider, initialize func() error) *Runtime {
	return &Runtime{
		policies:   make(map[string]*PolicyRuntime),
		wake:       make(chan struct{}, 1),
		done:       make(chan struct{}),
		syncStatus: "idle",
		hooks:      h,
		catalog:    provider,
		initialize: initialize,
	}
}

// Policy changes are rare and local: serialize them with one mutex.
func (rt *Runtime) lockPolicy(_ string) func() {
	rt.opMu.Lock()
	return rt.opMu.Unlock
}

func GetRuntime() *Runtime {
	globalMu.Lock()
	defer globalMu.Unlock()
	return global
}

func Stop() {
	rt := GetRuntime()
	if rt != nil {
		rt.Stop()
	}
}

func (rt *Runtime) Start(ctx context.Context) error {
	rt.startOnce.Do(func() { rt.startErr = rt.start(ctx) })
	return rt.startErr
}

func (rt *Runtime) start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	rt.ctx, rt.cancel = context.WithCancel(ctx)
	if err := rt.initialize(); err != nil {
		return err
	}
	rows, err := ListAppliedPolicies()
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := rt.loadPolicy(row); err != nil {
			return fmt.Errorf("clash: load policy %s/%d: %w", row.MasterGuid, row.ClientID, err)
		}
	}
	rt.resumeRemovals()
	rt.reconcileAll(false)
	rt.started = true
	rt.wg.Add(2)
	go rt.loop()
	go rt.syncLoop()
	return nil
}

func (rt *Runtime) Stop() {
	globalMu.Lock()
	if !rt.started {
		globalMu.Unlock()
		return
	}
	rt.started = false
	globalMu.Unlock()

	close(rt.done)
	if rt.cancel != nil {
		rt.cancel()
	}
	rt.wg.Wait()

	rt.mu.Lock()
	policies := make([]*PolicyRuntime, 0, len(rt.policies))
	for _, pr := range rt.policies {
		policies = append(policies, pr)
	}
	rt.policies = make(map[string]*PolicyRuntime)
	rt.mu.Unlock()
	for _, pr := range policies {
		if pr.server != nil {
			_ = pr.server.Close()
			pr.server = nil
		}
	}
}

func (rt *Runtime) loop() {
	defer rt.wg.Done()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	reconcile := time.NewTicker(5 * time.Second)
	defer reconcile.Stop()
	for {
		select {
		case <-rt.done:
			return
		case <-rt.wake:
			rt.reconcileAll(true)
		case <-tick.C:
			rt.tickPolicies()
		case <-reconcile.C:
			rt.resumeRemovals()
			rt.reconcileAll(true)
		}
	}
}

func (rt *Runtime) syncLoop() {
	defer rt.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-rt.done:
			return
		case <-ticker.C:
			rt.SyncOnce(rt.ctx)
		}
	}
}

func (rt *Runtime) tickPolicies() {
	rt.mu.RLock()
	policies := make([]*PolicyRuntime, 0, len(rt.policies))
	for _, pr := range rt.policies {
		policies = append(policies, pr)
	}
	rt.mu.RUnlock()
	for _, pr := range policies {
		if pr.policy != nil {
			pr.policy.Tick()
		}
	}
}

func (rt *Runtime) enqueue(_ *PolicyRuntime, _ gateway.StateChange) {
	select {
	case rt.wake <- struct{}{}:
	default:
	}
}

func (rt *Runtime) catalogNow() (*catalog.Catalog, error) {
	if rt.catalog == nil {
		return nil, errors.New("clash: catalog provider is not configured")
	}
	return rt.catalog()
}

func (rt *Runtime) reconcileAll(apply bool) {
	cat, err := rt.catalogNow()
	if err != nil {
		logger.Warningf("clash: catalog unavailable: %v", err)
		return
	}
	rt.mu.RLock()
	policies := make([]*PolicyRuntime, 0, len(rt.policies))
	for _, pr := range rt.policies {
		policies = append(policies, pr)
	}
	rt.mu.RUnlock()
	for _, pr := range policies {
		rt.reconcilePolicy(pr, cat, apply)
	}
}

func policyKey(masterGuid string, clientID int) string {
	return masterGuid + "|" + strconv.Itoa(clientID)
}

func (rt *Runtime) findPolicy(masterGuid string, clientID int) *PolicyRuntime {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.policies[policyKey(masterGuid, clientID)]
}

func (rt *Runtime) loadPolicy(row AppliedPolicy) error {
	if row.Removed {
		return nil
	}
	if row.BudgetBytes < 0 || row.Revision <= 0 {
		return fmt.Errorf("invalid budget/revision (%d/%d)", row.BudgetBytes, row.Revision)
	}
	pr := &PolicyRuntime{
		key:         policyKey(row.MasterGuid, row.ClientID),
		rowID:       row.ID,
		masterGuid:  row.MasterGuid,
		clientID:    row.ClientID,
		clientEmail: row.ClientEmail,
		revision:    row.Revision,
		ratio:       row.EffectiveRatio,
		budget:      row.BudgetBytes,
		graceSecs:   row.GraceSeconds,
		gracePct:    row.GracePercent,
		timezone:    row.PeriodTimezone,
		port:        row.GatewayPort,
		managed:     row.ManagedInbounds(),
		applied:     row.Applied,
		applyError:  row.ApplyError,
		removing:    row.Removing,
	}
	store := ledgerStore{rowID: row.ID}
	cb := gateway.Callbacks{OnStateChange: func(change gateway.StateChange) {
		rt.enqueue(pr, change)
	}}
	policy, err := gateway.NewPolicy(pr.key, gateway.Config{
		BudgetBytes:   row.BudgetBytes,
		GraceSeconds:  row.GraceSeconds,
		GracePercent:  row.GracePercent,
		ResetSchedule: row.ResetSchedule, AllocationRequired: row.AllocationRequired,
		PeriodTimezone: row.PeriodTimezone,
	}, AppliedPolicyByGatewayState(row), store, cb, time.Now)
	if err != nil {
		return err
	}
	pr.policy = policy
	rt.mu.Lock()
	rt.policies[pr.key] = pr
	rt.mu.Unlock()
	if row.Removing {
		return nil
	}
	if err := rt.startServer(pr); err != nil {
		pr.mu.Lock()
		pr.applyError = err.Error()
		pr.applied = false
		pr.mu.Unlock()
		_ = UpdateAppliedInbounds(pr.rowID, pr.managed, false, err.Error())
		logger.Warningf("clash: gateway %s: %v", pr.key, err)
	}
	return nil
}

type ledgerStore struct {
	rowID int
}

func (s ledgerStore) Save(st gateway.PersistedState) error {
	return SaveAppliedLedger(s.rowID, st)
}

func (rt *Runtime) startServer(pr *PolicyRuntime) error {
	if pr.server != nil {
		return nil
	}
	if pr.policy == nil {
		return errors.New("gateway policy is not initialized")
	}
	// The OS allocates the loopback port; no scanning or retry range.
	srv, err := gateway.NewServer(pr.policy, "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := srv.Addr().(*net.TCPAddr).Port
	db, err := storage.DB()
	if err == nil {
		err = db.Model(&AppliedPolicy{}).Where("id = ?", pr.rowID).Update("gateway_port", port).Error
	}
	if err != nil {
		_ = srv.DiscardListener()
		return err
	}
	pr.mu.Lock()
	pr.port, pr.server, pr.needsApply = port, srv, true
	pr.mu.Unlock()
	go srv.Serve()
	return nil
}

func (rt *Runtime) reconcilePolicy(pr *PolicyRuntime, cat *catalog.Catalog, apply bool) {
	if pr == nil || pr.policy == nil {
		return
	}
	unlock := rt.lockPolicy(pr.key)
	defer unlock()
	rt.reconcilePolicyLocked(pr, cat, apply)
}

func (rt *Runtime) reconcilePolicyLocked(pr *PolicyRuntime, cat *catalog.Catalog, apply bool) {
	if pr == nil || pr.policy == nil {
		return
	}
	pr.mu.Lock()
	if pr.removing {
		pr.mu.Unlock()
		return
	}
	clientEmail := pr.clientEmail
	listening := pr.server != nil
	pr.mu.Unlock()

	current := resolveManaged(cat, clientEmail)

	var applyErr []string
	if !listening {
		applyErr = append(applyErr, "gateway is not listening")
	}
	if err := rt.tagConflict(pr.masterGuid, pr.clientID, clientEmail, managedTags(current)); err != nil {
		applyErr = append(applyErr, err.Error())
	}

	if pr.managedChanged(current) {
		pr.mu.Lock()
		pr.managed = current
		pr.needsApply = true
		pr.mu.Unlock()
	}

	pending := pr.takeNeedsApply()
	needApply := apply && pending
	if !apply && pending {
		pr.mu.Lock()
		pr.needsApply = true
		pr.mu.Unlock()
	}
	if needApply && rt.hooks.ApplyXray != nil {
		if err := rt.hooks.ApplyXray(); err != nil {
			applyErr = append(applyErr, "apply xray: "+err.Error())
			pr.mu.Lock()
			pr.needsApply = true
			pr.mu.Unlock()
		}
	}

	if len(current) == 0 {
		applyErr = append(applyErr, "no local inbound carries this client")
	}
	pr.mu.Lock()
	pr.managed = current
	pr.applied = listening && len(current) > 0 && len(applyErr) == 0
	if len(applyErr) > 0 {
		pr.applyError = strings.Join(applyErr, "; ")
	} else {
		pr.applyError = ""
	}
	managedCopy := append([]ManagedInbound(nil), current...)
	applied := pr.applied
	errText := pr.applyError
	pr.mu.Unlock()

	if err := UpdateAppliedInbounds(pr.rowID, managedCopy, applied, errText); err != nil {
		pr.mu.Lock()
		pr.applied = false
		pr.applyError = "persist inbound result: " + err.Error()
		pr.mu.Unlock()
		logger.Warningf("clash: persist inbounds for %s: %v", pr.key, err)
	}
}

func (pr *PolicyRuntime) managedChanged(next []ManagedInbound) bool {
	pr.mu.Lock()
	defer pr.mu.Unlock()
	if len(next) != len(pr.managed) {
		return true
	}
	for i := range next {
		if next[i].Tag != pr.managed[i].Tag || next[i].Port != pr.managed[i].Port ||
			next[i].Enabled != pr.managed[i].Enabled {
			return true
		}
	}
	return false
}

func resolveManaged(cat *catalog.Catalog, email string) []ManagedInbound {
	out := make([]ManagedInbound, 0)
	for _, in := range cat.Inbounds {
		if in.NodeKey != catalog.LocalNodeKey {
			continue
		}
		if !containsString(in.Clients, email) {
			continue
		}
		if !in.Enabled {
			continue
		}
		m := ManagedInbound{
			ID:       in.ID,
			Tag:      in.Tag,
			Protocol: in.Protocol,
			Listen:   in.Listen,
			Port:     in.Port,
			Network:  in.Network,
			Security: in.Security,
			Enabled:  in.Enabled,
		}
		out = append(out, m)
	}
	return out
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
