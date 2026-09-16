package managedenv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Manager struct {
	root, target, coreID string
	catalog              Catalog
	client               *http.Client
	allowFile            bool
	timeout              time.Duration
	now                  func() time.Time
	ctx                  context.Context
	cancel               context.CancelFunc
	wg                   sync.WaitGroup
	mu                   sync.Mutex
	closed               bool
	calls                map[string]*call
	cacheMu              sync.Mutex
	manifests            map[string]cachedManifest
	health               map[string]cachedHealth
	healthGeneration     uint64
	// Override only in storage-fault tests; production uses atomic JSON writes.
	persistRecord func(string, any) error
}
type call struct {
	done     chan struct{}
	manifest Manifest
	err      error
}
type coreRecord struct {
	ID              string `json:"id"`
	PID             int    `json:"pid"`
	ProcessIdentity string `json:"process_identity"`
	Closed          bool   `json:"closed"`
}
type waiter struct {
	ID, CoreID string
	Owner      Owner
}

var defaultManager atomic.Pointer[Manager]

func SetDefault(m *Manager) { defaultManager.Store(m) }
func Default() *Manager     { return defaultManager.Load() }
func FromContext(ctx context.Context) *Manager {
	if m, ok := ctx.Value(managerContextKey{}).(*Manager); ok {
		return m
	}
	return Default()
}
func newID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func Target() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return runtime.GOOS + "-" + arch
}

func New(options Options) (*Manager, error) {
	if !filepath.IsAbs(options.Root) || strings.TrimSpace(options.Root) == "" {
		return nil, failure("environment_root_invalid", "an explicit absolute AWAKEN_HOME is required")
	}
	root := filepath.Join(filepath.Clean(options.Root), "runtime", "environments")
	if e := ownedPath(filepath.Clean(options.Root), root); e != nil {
		return nil, e
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	canonical, e := filepath.EvalSymlinks(root)
	if e != nil {
		return nil, e
	}
	root = canonical
	if options.Catalog.SchemaVersion != SchemaVersion {
		return nil, failure("environment_schema_unsupported", "catalog schema is unsupported")
	}
	if options.Target == "" {
		options.Target = Target()
	}
	if options.Client == nil {
		options.Client = &http.Client{Timeout: 10 * time.Minute}
	}
	if options.Timeout == 0 {
		options.Timeout = 20 * time.Minute
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{root: root, target: options.Target, coreID: newID(), catalog: options.Catalog, client: options.Client, allowFile: options.AllowFileAssets, timeout: options.Timeout, now: options.Now, ctx: ctx, cancel: cancel, calls: map[string]*call{}}
	for _, d := range []string{"components", "environments", "operations", "waiters", "cores", "leases", "bindings", "removals", "component-uses", "locks", "cache/downloads", "cache/python", "cache/npm", "staging"} {
		if e := ownedPath(root, filepath.Join(root, d)); e != nil {
			cancel()
			return nil, e
		}
		if e := os.MkdirAll(filepath.Join(root, d), 0700); e != nil {
			cancel()
			return nil, e
		}
	}
	if e := m.registry(func(*registry) error { return nil }); e != nil {
		cancel()
		return nil, e
	}
	if e := writeJSON(filepath.Join(root, "cores", m.coreID+".json"), coreRecord{ID: m.coreID, PID: os.Getpid(), ProcessIdentity: processIdentity(os.Getpid())}); e != nil {
		cancel()
		return nil, e
	}
	m.wg.Add(1)
	go m.maintainLoop()
	return m, nil
}
func (m *Manager) Root() string     { return m.root }
func (m *Manager) Catalog() Catalog { b := clone(m.catalog); return b }
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.cancel()
	m.mu.Unlock()
	m.wg.Wait()
	return writeJSON(filepath.Join(m.root, "cores", m.coreID+".json"), coreRecord{ID: m.coreID, PID: os.Getpid(), ProcessIdentity: processIdentity(os.Getpid()), Closed: true})
}
func (m *Manager) coreAlive(id string) bool {
	var r coreRecord
	return safeID.MatchString(id) && readJSON(filepath.Join(m.root, "cores", id+".json"), &r) == nil && !r.Closed && r.ID == id && processAlive(r.PID) && r.ProcessIdentity != "" && r.ProcessIdentity == processIdentity(r.PID)
}
func (m *Manager) activeWaiters(key string) bool {
	entries, _ := os.ReadDir(filepath.Join(m.root, "waiters", key))
	for _, e := range entries {
		var w waiter
		if readJSON(filepath.Join(m.root, "waiters", key, e.Name()), &w) == nil && m.coreAlive(w.CoreID) {
			return true
		}
	}
	return false
}
func (m *Manager) Resolve(ctx context.Context, req Request) (Manifest, error) {
	if e := ctx.Err(); e != nil {
		return Manifest{}, e
	}
	s, e := m.catalog.Resolve(req)
	if e != nil {
		return Manifest{}, e
	}
	return m.resolveSpec(s)
}
func (m *Manager) resolveSpec(s Spec) (Manifest, error) {
	f, err := m.findSpec(s)
	var missing *Error
	if !errors.As(err, &missing) || missing.Code != "environment_missing" {
		return f, err
	}
	// A finished manifest can outlive an interrupted index commit. Recovery is
	// needed on a miss, not on every successful lookup or binding refresh.
	if err := m.recoverRegistry(); err != nil {
		return Manifest{}, err
	}
	return m.findSpec(s)
}

func (m *Manager) findSpec(s Spec) (Manifest, error) {
	all, e := m.instances()
	if e != nil {
		return Manifest{}, e
	}
	for _, f := range all {
		if (f.Kind != "environment" && f.Kind != "office") || !specContains(f.Spec, s) || !m.reusable(f) {
			continue
		}
		healthy := true
		for _, id := range f.Components {
			found := false
			for _, c := range all {
				if c.InstanceID == id && m.reusable(c) {
					found = true
					break
				}
			}
			healthy = healthy && found
		}
		if healthy && (f.Kind != "office" || m.officeSelected(f)) {
			return f, nil
		}
	}
	return Manifest{}, failure("environment_missing", "compatible shared environment is not ready")
}
func (m *Manager) Ensure(ctx context.Context, req Request, owner Owner, progress Progress) (binding Binding, err error) {
	defer func() { err = classifyPreparationError(err) }()
	if !owner.valid() {
		return Binding{}, failure("environment_owner_invalid", "host execution identity is required")
	}
	m.invalidateHealth()
	if req.EnvironmentRef != "" {
		b, e := m.Binding(ctx, req.EnvironmentRef, owner)
		if req.Capability == "" && len(req.Requirements) == 0 && e != nil {
			return Binding{}, e
		}
		if e == nil {
			if req.Capability == "" && len(req.Requirements) == 0 {
				return b, nil
			}
			s, err := m.catalog.Resolve(req)
			if err == nil && specContains(b.Spec, s) {
				return b, nil
			}
		}
	}
	s, e := m.catalog.Resolve(req)
	if e != nil {
		return Binding{}, e
	}
	return m.ensureSpec(ctx, s, owner, progress)
}

func (m *Manager) ensureSpec(ctx context.Context, s Spec, owner Owner, progress Progress) (Binding, error) {
	return m.prepareSpec(ctx, s, owner, progress, "")
}

func (m *Manager) prepareSpec(ctx context.Context, s Spec, owner Owner, progress Progress, repairOf string) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	if !owner.valid() {
		return Binding{}, failure("environment_owner_invalid", "host execution identity is required")
	}
	if repairOf == "" {
		for _, existing := range m.Bindings(owner) {
			if specContains(existing.Spec, s) {
				return m.Binding(ctx, existing.EnvironmentRef, owner)
			}
		}
		if f, e := m.resolveSpec(s); e == nil {
			return m.bind(f, owner, true)
		} else {
			var typed *Error
			if !errors.As(e, &typed) || typed.Code != "environment_missing" {
				return Binding{}, e
			}
		}
	}
	groupKey := digest([]any{m.target, s})
	key := groupKey
	if repairOf != "" {
		key = digest([]any{m.target, s, "repair", repairOf})
	}
	w := waiter{ID: newID(), CoreID: m.coreID, Owner: owner}
	wp := filepath.Join(m.root, "waiters", groupKey, w.ID+".json")
	if e := writeJSON(wp, w); e != nil {
		return Binding{}, e
	}
	defer os.Remove(wp)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Binding{}, context.Canceled
	}
	c := m.calls[key]
	if c == nil {
		c = &call{done: make(chan struct{})}
		m.calls[key] = c
		m.wg.Add(1)
		go m.prepare(key, s, c, repairOf)
	}
	m.mu.Unlock()
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	last := ""
	for {
		select {
		case <-ctx.Done():
			return Binding{}, ctx.Err()
		case <-m.ctx.Done():
			return Binding{}, context.Canceled
		case <-ticker.C:
			if progress != nil {
				if op, e := m.Operation(key); e == nil {
					// A previous executor's terminal record is not this waiter's
					// terminal state while takeover is still running.
					if operationTerminal(op.State) {
						select {
						case <-c.done:
						default:
							op.State, op.Message, op.ErrorCode = "waiting", "正在等待共享环境准备或恢复", ""
						}
					}
					d := digest(op)
					if d != last {
						progress(op)
						last = d
					}
				}
			}
		case <-c.done:
			if progress != nil {
				if op, e := m.Operation(key); e == nil {
					progress(op)
				}
			}
			if c.err != nil {
				return Binding{}, c.err
			}
			if err := ctx.Err(); err != nil {
				return Binding{}, err
			}
			return m.bind(c.manifest, owner, false)
		}
	}
}
func (m *Manager) prepare(key string, s Spec, c *call, repairOf string) {
	defer m.wg.Done()
	defer func() { m.mu.Lock(); delete(m.calls, key); close(c.done); m.mu.Unlock() }()
	ctx, cancel := context.WithTimeout(m.ctx, m.timeout)
	defer cancel()
	groupKey := digest([]any{m.target, s})
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		timer := time.NewTicker(200 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-ctx.Done():
				return
			case <-timer.C:
				if !m.activeWaiters(groupKey) {
					cancel()
					return
				}
			}
		}
	}()
	lockKey := key
	if s.OfficeBundle != nil {
		lockKey = groupKey
	}
	c.err = m.locked(ctx, "prepare-"+lockKey, func() error {
		if repairOf != "" {
			selected, err := m.officeChoice()
			if err != nil {
				return err
			}
			if selected.Current != repairOf {
				f, err := m.resolveSpec(s)
				if err != nil {
					return failure("environment_state_changed", "办公环境状态已变化，请刷新后再试")
				}
				c.manifest = f
				return nil
			}
		} else {
			if f, e := m.resolveSpec(s); e == nil {
				c.manifest = f
				return nil
			} else {
				var typed *Error
				if !errors.As(e, &typed) || typed.Code != "environment_missing" {
					return e
				}
			}
		}
		op := Operation{RepairOf: repairOf, ID: key, RequestKey: key, Attempt: newID(), CoreID: m.coreID, State: "resolving", Message: "正在检查共享环境"}
		if s.OfficeBundle != nil {
			op.BundleVersion = s.OfficeBundle.Version
			op.Component = "office-bundle"
		}
		if e := m.record(&op); e != nil {
			return e
		}
		f, e := m.install(ctx, s, &op)
		if e != nil {
			e = classifyPreparationError(e)
			op.State = "failed"
			op.Message = "运行环境准备失败"
			var typed *Error
			if errors.As(e, &typed) {
				op.ErrorCode = typed.Code
			} else {
				op.ErrorCode = "environment_prepare_failed"
			}
			if ctx.Err() != nil {
				op.State = "interrupted"
				op.ErrorCode = "environment_prepare_interrupted"
			}
			_ = m.record(&op)
			return e
		}
		c.manifest = f
		op.State = "ready"
		op.Message = "运行环境已就绪"
		return m.record(&op)
	})
}
func (m *Manager) record(op *Operation) error {
	op.UpdatedAt = m.now()
	path := filepath.Join(m.root, "operations", op.ID+".json")
	if op.started {
		var current Operation
		if err := readJSON(path, &current); err == nil && (current.Attempt != op.Attempt || current.CoreID != op.CoreID) {
			return failure("environment_attempt_superseded", "an old preparation attempt cannot publish")
		}
	}
	if err := writeJSON(path, op); err != nil {
		return err
	}
	op.started = true
	return writeJSON(filepath.Join(m.root, "attempts", op.ID, op.Attempt+".json"), op)
}
func (m *Manager) Operation(id string) (Operation, error) {
	var op Operation
	if len(id) != 64 || !safeID.MatchString(id) {
		return op, failure("environment_operation_invalid", "invalid operation ID")
	}
	e := readJSON(filepath.Join(m.root, "operations", id+".json"), &op)
	return op, e
}
func (m *Manager) Operations() []Operation {
	paths, _ := filepath.Glob(filepath.Join(m.root, "attempts", "*", "*.json"))
	out := []Operation{}
	seen := map[string]bool{}
	for _, p := range paths {
		var op Operation
		if readJSON(p, &op) == nil {
			out = append(out, op)
			seen[op.ID] = true
		}
	}
	paths, _ = filepath.Glob(filepath.Join(m.root, "operations", "*.json"))
	for _, p := range paths {
		var op Operation
		if readJSON(p, &op) == nil && !seen[op.ID] {
			out = append(out, op)
		}
	}
	return out
}
