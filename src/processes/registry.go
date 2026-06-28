package processes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"awaken/src/internal/outputlimit"
	"awaken/src/tool"
)

const (
	ModePipe = "pipe"

	StatusRunning = "running"
	StatusExited  = "exited"
	StatusKilled  = "killed"
	StatusFailed  = "failed"

	DefaultMaxLogChars = 200000
	MaxLogCharsLimit   = 1000000
)

var (
	ErrSessionNotFound = errors.New("process session not found")
	ErrSessionExited   = errors.New("process session is not running")

	DefaultRegistry = NewRegistry(RegistryConfig{})
)

type RegistryConfig struct {
	MaxLogChars int
	Now         func() time.Time
	NewID       func() string
}

type Registry struct {
	mu          sync.RWMutex
	sessions    map[string]*session
	maxLogChars int
	now         func() time.Time
	newID       func() string
}

type StartOptions struct {
	Command           string
	Workdir           string
	EnvAllowlist      []string
	Background        bool
	MaxLogChars       int
	PermissionContext tool.PermissionContext
}

type SessionSnapshot struct {
	ID            string     `json:"id"`
	Mode          string     `json:"mode"`
	Command       string     `json:"command"`
	Workdir       string     `json:"workdir"`
	EnvAllowlist  []string   `json:"env_allowlist,omitempty"`
	PID           int        `json:"pid,omitempty"`
	Status        string     `json:"status"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	Error         string     `json:"error,omitempty"`
	StartedAt     time.Time  `json:"started_at"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	DurationMS    int64      `json:"duration_ms"`
	LogTruncated  bool       `json:"log_truncated"`
	MaxLogChars   int        `json:"max_log_chars"`
	KillRequested bool       `json:"kill_requested,omitempty"`
}

type ProcessLog struct {
	ID                string `json:"id"`
	Mode              string `json:"mode"`
	Status            string `json:"status"`
	Stdout            string `json:"stdout"`
	Stderr            string `json:"stderr"`
	Combined          string `json:"combined"`
	StdoutTruncated   bool   `json:"stdout_truncated"`
	StderrTruncated   bool   `json:"stderr_truncated"`
	CombinedTruncated bool   `json:"combined_truncated"`
	Truncated         bool   `json:"truncated"`
}

type WaitResult struct {
	Session  SessionSnapshot `json:"session"`
	TimedOut bool            `json:"timed_out"`
}

type session struct {
	id           string
	mode         string
	command      string
	workdir      string
	envAllowlist []string
	pid          int

	cmd       *exec.Cmd
	cancel    context.CancelFunc
	stdin     io.WriteCloser
	stdinMu   sync.Mutex
	waitDone  chan struct{}
	readerWG  sync.WaitGroup
	logs      *processLogs
	startedAt time.Time

	mu            sync.RWMutex
	status        string
	exitCode      *int
	errMessage    string
	endedAt       *time.Time
	killRequested bool
}

type processLogs struct {
	mu       sync.Mutex
	maxChars int
	stdout   *outputlimit.CappedBuffer
	stderr   *outputlimit.CappedBuffer
	combined *outputlimit.CappedBuffer
}

func NewRegistry(config RegistryConfig) *Registry {
	maxLogChars := normalizeLogChars(config.MaxLogChars, DefaultMaxLogChars)
	now := config.Now
	if now == nil {
		now = time.Now
	}
	newID := config.NewID
	if newID == nil {
		newID = newProcessID
	}
	return &Registry{
		sessions:    make(map[string]*session),
		maxLogChars: maxLogChars,
		now:         now,
		newID:       newID,
	}
}

func (r *Registry) Start(ctx context.Context, options StartOptions) (SessionSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SessionSnapshot{}, err
	}

	command := strings.TrimSpace(options.Command)
	if command == "" {
		return SessionSnapshot{}, fmt.Errorf("command is required")
	}

	permissionContext := options.PermissionContext.Normalize()
	workdir, err := resolveWorkdir(options.Workdir, permissionContext)
	if err != nil {
		return SessionSnapshot{}, err
	}

	processCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(processCtx, "/bin/sh", "-c", command)
	configureCommand(cmd)
	cmd.Dir = workdir
	cmd.Env = buildProcessEnv(workdir, options.EnvAllowlist)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return SessionSnapshot{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return SessionSnapshot{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return SessionSnapshot{}, err
	}

	maxLogChars := normalizeLogChars(options.MaxLogChars, r.maxLogChars)
	startedAt := r.now().UTC()
	managed := &session{
		id:           r.newID(),
		mode:         ModePipe,
		command:      command,
		workdir:      workdir,
		envAllowlist: sanitizeEnvAllowlist(options.EnvAllowlist),
		cmd:          cmd,
		cancel:       cancel,
		stdin:        stdin,
		waitDone:     make(chan struct{}),
		logs:         newProcessLogs(maxLogChars),
		startedAt:    startedAt,
		status:       StatusRunning,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return SessionSnapshot{}, err
	}
	if cmd.Process != nil {
		managed.pid = cmd.Process.Pid
	}

	r.mu.Lock()
	r.sessions[managed.id] = managed
	r.mu.Unlock()

	managed.startReaders(stdout, stderr)
	go managed.wait(r.now)

	if options.Background {
		return managed.snapshot(r.now), nil
	}

	waited, err := r.Wait(ctx, managed.id, 0)
	if err != nil {
		_, _ = r.Kill(context.Background(), managed.id)
		return managed.snapshot(r.now), err
	}
	return waited.Session, nil
}

func (r *Registry) List() []SessionSnapshot {
	r.mu.RLock()
	sessions := make([]*session, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.mu.RUnlock()

	snapshots := make([]SessionSnapshot, 0, len(sessions))
	for _, session := range sessions {
		snapshots = append(snapshots, session.snapshot(r.now))
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].StartedAt.Equal(snapshots[j].StartedAt) {
			return snapshots[i].ID < snapshots[j].ID
		}
		return snapshots[i].StartedAt.Before(snapshots[j].StartedAt)
	})
	return snapshots
}

func (r *Registry) GetLog(sessionID string, maxChars int) (ProcessLog, error) {
	session, err := r.get(sessionID)
	if err != nil {
		return ProcessLog{}, err
	}
	log := session.logs.result(maxChars)
	log.ID = session.id
	log.Mode = session.mode
	log.Status = session.statusSnapshot()
	return log, nil
}

func (r *Registry) WriteStdin(ctx context.Context, sessionID string, data string) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	session, err := r.get(sessionID)
	if err != nil {
		return 0, err
	}
	if !session.running() {
		return 0, ErrSessionExited
	}

	session.stdinMu.Lock()
	defer session.stdinMu.Unlock()
	if !session.running() {
		return 0, ErrSessionExited
	}
	return io.WriteString(session.stdin, data)
}

func (r *Registry) Kill(ctx context.Context, sessionID string) (SessionSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SessionSnapshot{}, err
	}
	session, err := r.get(sessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if !session.running() {
		return session.snapshot(r.now), nil
	}

	session.requestKill()
	_ = signalTerminate(session.cmd)

	select {
	case <-session.waitDone:
		return session.snapshot(r.now), nil
	case <-time.After(2 * time.Second):
	}

	_ = signalKill(session.cmd)
	session.cancel()

	select {
	case <-session.waitDone:
		return session.snapshot(r.now), nil
	case <-ctx.Done():
		return session.snapshot(r.now), ctx.Err()
	case <-time.After(2 * time.Second):
		return session.snapshot(r.now), fmt.Errorf("process %s did not exit after kill", sessionID)
	}
}

func (r *Registry) Wait(ctx context.Context, sessionID string, timeout time.Duration) (WaitResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := r.get(sessionID)
	if err != nil {
		return WaitResult{}, err
	}

	waitCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	select {
	case <-session.waitDone:
		return WaitResult{Session: session.snapshot(r.now)}, nil
	case <-waitCtx.Done():
		if timeout > 0 && errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return WaitResult{Session: session.snapshot(r.now), TimedOut: true}, nil
		}
		return WaitResult{Session: session.snapshot(r.now)}, waitCtx.Err()
	}
}

func (r *Registry) get(sessionID string) (*session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, ErrSessionNotFound
	}
	r.mu.RLock()
	session := r.sessions[sessionID]
	r.mu.RUnlock()
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func (s *session) startReaders(stdout io.Reader, stderr io.Reader) {
	s.readerWG.Add(2)
	go s.readStream("stdout", stdout)
	go s.readStream("stderr", stderr)
}

func (s *session) readStream(stream string, reader io.Reader) {
	defer s.readerWG.Done()
	buf := make([]byte, 32768)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			s.logs.write(stream, buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *session) wait(now func() time.Time) {
	err := s.cmd.Wait()
	s.readerWG.Wait()

	status := StatusExited
	var exitCode *int
	var errMessage string
	if s.killRequestedSnapshot() {
		status = StatusKilled
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			exitCode = &code
		} else {
			status = StatusFailed
			errMessage = err.Error()
		}
	} else {
		code := 0
		exitCode = &code
	}

	endedAt := now().UTC()
	s.mu.Lock()
	s.status = status
	s.exitCode = exitCode
	s.errMessage = errMessage
	s.endedAt = &endedAt
	s.mu.Unlock()
	close(s.waitDone)
}

func (s *session) running() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status == StatusRunning
}

func (s *session) statusSnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *session) requestKill() {
	s.mu.Lock()
	s.killRequested = true
	s.mu.Unlock()
}

func (s *session) killRequestedSnapshot() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.killRequested
}

func (s *session) snapshot(now func() time.Time) SessionSnapshot {
	s.mu.RLock()
	status := s.status
	var exitCode *int
	if s.exitCode != nil {
		code := *s.exitCode
		exitCode = &code
	}
	errMessage := s.errMessage
	var endedAt *time.Time
	if s.endedAt != nil {
		end := *s.endedAt
		endedAt = &end
	}
	killRequested := s.killRequested
	s.mu.RUnlock()

	end := now().UTC()
	if endedAt != nil {
		end = *endedAt
	}
	duration := end.Sub(s.startedAt)
	if duration < 0 {
		duration = 0
	}

	return SessionSnapshot{
		ID:            s.id,
		Mode:          s.mode,
		Command:       s.command,
		Workdir:       s.workdir,
		EnvAllowlist:  append([]string(nil), s.envAllowlist...),
		PID:           s.pid,
		Status:        status,
		ExitCode:      exitCode,
		Error:         errMessage,
		StartedAt:     s.startedAt,
		EndedAt:       endedAt,
		DurationMS:    duration.Milliseconds(),
		LogTruncated:  s.logs.truncated(),
		MaxLogChars:   s.logs.maxCharsValue(),
		KillRequested: killRequested,
	}
}

func newProcessLogs(maxChars int) *processLogs {
	return &processLogs{
		maxChars: maxChars,
		stdout:   outputlimit.NewCappedBuffer(maxChars),
		stderr:   outputlimit.NewCappedBuffer(maxChars),
		combined: outputlimit.NewCappedBuffer(maxChars),
	}
}

func (l *processLogs) write(stream string, data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.combined.Write(data)
	switch stream {
	case "stderr":
		_, _ = l.stderr.Write(data)
	default:
		_, _ = l.stdout.Write(data)
	}
}

func (l *processLogs) result(maxChars int) ProcessLog {
	l.mu.Lock()
	defer l.mu.Unlock()

	stdout := limitLogResult(l.stdout.Result(), maxChars)
	stderr := limitLogResult(l.stderr.Result(), maxChars)
	combined := limitLogResult(l.combined.Result(), maxChars)
	return ProcessLog{
		Stdout:            stdout.Content,
		Stderr:            stderr.Content,
		Combined:          combined.Content,
		StdoutTruncated:   stdout.Truncated,
		StderrTruncated:   stderr.Truncated,
		CombinedTruncated: combined.Truncated,
		Truncated:         stdout.Truncated || stderr.Truncated || combined.Truncated,
	}
}

func (l *processLogs) truncated() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stdout.Result().Truncated || l.stderr.Result().Truncated || l.combined.Result().Truncated
}

func (l *processLogs) maxCharsValue() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.maxChars
}

func resolveWorkdir(workdir string, permissionContext tool.PermissionContext) (string, error) {
	trimmed := strings.TrimSpace(workdir)
	if trimmed == "" {
		trimmed = permissionContext.WorkspaceRootOrCWD()
	}
	resolved, err := permissionContext.ResolvePathChecked(trimmed)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("cannot stat workdir %q: %w", trimmed, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workdir %q is not a directory", trimmed)
	}
	return resolved, nil
}

func limitLogResult(result outputlimit.Result, maxChars int) outputlimit.Result {
	if maxChars <= 0 {
		return result
	}
	limited := outputlimit.Limit(result.Content, maxChars)
	limited.Truncated = limited.Truncated || result.Truncated
	return limited
}

func normalizeLogChars(value int, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value <= 0 {
		value = DefaultMaxLogChars
	}
	if value > MaxLogCharsLimit {
		value = MaxLogCharsLimit
	}
	return value
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var defaultEnvAllowlist = []string{
	"PATH",
	"HOME",
	"USER",
	"LOGNAME",
	"SHELL",
	"TMPDIR",
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"TERM",
	"COLORTERM",
}

func buildProcessEnv(workdir string, allowlist []string) []string {
	names := append([]string{}, defaultEnvAllowlist...)
	names = append(names, allowlist...)
	names = sanitizeEnvAllowlist(names)

	env := make([]string, 0, len(names)+1)
	seen := make(map[string]struct{}, len(names)+1)
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	if _, ok := seen["PATH"]; !ok || os.Getenv("PATH") == "" {
		env = append(env, "PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	}
	env = append(env, "PWD="+workdir)
	return env
}

func sanitizeEnvAllowlist(names []string) []string {
	clean := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		name = strings.ToUpper(strings.TrimSpace(name))
		if name == "" || !envNamePattern.MatchString(name) || isSensitiveEnvName(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		clean = append(clean, name)
	}
	return clean
}

func isSensitiveEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, marker := range []string{
		"SECRET",
		"TOKEN",
		"KEY",
		"PASSWORD",
		"PASS",
		"CREDENTIAL",
		"AUTH",
		"COOKIE",
		"SESSION",
		"OPENAI",
		"ANTHROPIC",
		"LLM",
		"API",
		"ACCESS",
		"PRIVATE",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}

var processIDCounter atomic.Uint64

func newProcessID() string {
	return fmt.Sprintf("proc_%d_%d", time.Now().UnixNano(), processIDCounter.Add(1))
}
