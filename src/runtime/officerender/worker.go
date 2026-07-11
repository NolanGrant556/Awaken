package officerender

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxConcurrency = 1
	DefaultTimeout        = 90 * time.Second
	maxCommandOutputBytes = 64 * 1024
)

var (
	ErrInvalidJob         = errors.New("invalid office render job")
	ErrRuntimeAttestation = errors.New("office render runtime attestation failed")
	ErrRenderTimeout      = errors.New("office render timed out")
	ErrNoOutput           = errors.New("office render produced no output")
)

// Runtime identifies one immutable managed-runtime revision.
type Runtime struct {
	Revision    string
	Root        string
	SOfficePath string
	Attestation string
}

// Job describes one LibreOffice conversion and its final atomic destination.
type Job struct {
	Runtime    Runtime
	InputPath  string
	OutputPath string
	Format     string
}

type Result struct {
	OutputPath      string
	RuntimeRevision string
	Duration        time.Duration
	Shared          bool
}

type RuntimeAttestor func(context.Context, Runtime) error
type RuntimeResolver func(context.Context) (Runtime, error)

type Config struct {
	MaxConcurrent  int
	Timeout        time.Duration
	TempRoot       string
	Environment    []string
	ResolveRuntime RuntimeResolver
	Attest         RuntimeAttestor
}

type renderFlight struct {
	done     chan struct{}
	cancel   context.CancelFunc
	waiters  int
	finished bool
	result   Result
	err      error
}

type attestationFlight struct {
	done        chan struct{}
	fingerprint string
	err         error
}

// OfficeRenderWorker serializes LibreOffice by default while deduplicating
// identical concurrent conversions. It is safe for concurrent use.
type OfficeRenderWorker struct {
	timeout        time.Duration
	tempRoot       string
	environment    []string
	resolveRuntime RuntimeResolver
	attest         RuntimeAttestor
	semaphore      chan struct{}

	flightMu sync.Mutex
	flights  map[string]*renderFlight

	attestationMu      sync.Mutex
	attestedRevisions  map[string]string
	attestationFlights map[string]*attestationFlight
}

func NewOfficeRenderWorker(config Config) *OfficeRenderWorker {
	maxConcurrent := config.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrency
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &OfficeRenderWorker{
		timeout:            timeout,
		tempRoot:           strings.TrimSpace(config.TempRoot),
		environment:        append([]string(nil), config.Environment...),
		resolveRuntime:     config.ResolveRuntime,
		attest:             config.Attest,
		semaphore:          make(chan struct{}, maxConcurrent),
		flights:            make(map[string]*renderFlight),
		attestedRevisions:  make(map[string]string),
		attestationFlights: make(map[string]*attestationFlight),
	}
}

func (w *OfficeRenderWorker) Render(ctx context.Context, job Job) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("%w: context is nil", ErrInvalidJob)
	}
	if w.resolveRuntime != nil {
		resolvedRuntime, err := w.resolveRuntime(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("resolve managed office runtime: %w", err)
		}
		job.Runtime = resolvedRuntime
	}
	normalized, inputInfo, err := normalizeJob(job)
	if err != nil {
		return Result{}, err
	}
	key := renderKey(normalized, inputInfo)

	w.flightMu.Lock()
	if existing := w.flights[key]; existing != nil {
		existing.waiters++
		w.flightMu.Unlock()
		return w.waitForFlight(ctx, existing, true)
	}
	flightCtx, cancel := context.WithCancel(context.Background())
	flight := &renderFlight{done: make(chan struct{}), cancel: cancel, waiters: 1}
	w.flights[key] = flight
	w.flightMu.Unlock()
	go w.runFlight(flightCtx, key, flight, normalized)
	return w.waitForFlight(ctx, flight, false)
}

func (w *OfficeRenderWorker) runFlight(ctx context.Context, key string, flight *renderFlight, job Job) {
	result, renderErr := w.renderOnce(ctx, job)
	w.flightMu.Lock()
	flight.result = result
	flight.err = renderErr
	flight.finished = true
	close(flight.done)
	delete(w.flights, key)
	w.flightMu.Unlock()
	flight.cancel()
}

func (w *OfficeRenderWorker) waitForFlight(ctx context.Context, flight *renderFlight, shared bool) (Result, error) {
	select {
	case <-flight.done:
		result := flight.result
		result.Shared = shared
		return result, flight.err
	default:
	}
	select {
	case <-flight.done:
		result := flight.result
		result.Shared = shared
		return result, flight.err
	case <-ctx.Done():
		w.flightMu.Lock()
		if !flight.finished {
			flight.waiters--
			if flight.waiters == 0 {
				flight.cancel()
			}
		}
		w.flightMu.Unlock()
		return Result{}, ctx.Err()
	}
}

func (w *OfficeRenderWorker) InvalidateRuntimeRevision(revision string) {
	w.attestationMu.Lock()
	delete(w.attestedRevisions, strings.TrimSpace(revision))
	w.attestationMu.Unlock()
}

func (w *OfficeRenderWorker) renderOnce(ctx context.Context, job Job) (Result, error) {
	started := time.Now()
	if err := w.ensureRuntimeAttested(ctx, job.Runtime); err != nil {
		return Result{}, err
	}
	if err := w.acquire(ctx); err != nil {
		return Result{}, err
	}
	defer w.release()

	destinationDir := filepath.Dir(job.OutputPath)
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return Result{}, fmt.Errorf("create office render destination: %w", err)
	}
	stagingRoot, err := os.MkdirTemp(destinationDir, ".awaken-office-render-")
	if err != nil {
		return Result{}, fmt.Errorf("create office render staging directory: %w", err)
	}
	defer os.RemoveAll(stagingRoot)
	stagingOutput := filepath.Join(stagingRoot, "output")
	if err := os.Mkdir(stagingOutput, 0o755); err != nil {
		return Result{}, fmt.Errorf("create office render output staging directory: %w", err)
	}

	tempRoot := w.tempRoot
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		return Result{}, fmt.Errorf("create office render temp root: %w", err)
	}
	profileDir, err := os.MkdirTemp(tempRoot, "awaken-libreoffice-profile-")
	if err != nil {
		return Result{}, fmt.Errorf("create isolated LibreOffice profile: %w", err)
	}
	defer os.RemoveAll(profileDir)
	stagedInput, err := stageInputFile(job.InputPath, stagingRoot)
	if err != nil {
		return Result{}, fmt.Errorf("stage office render input: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	args := []string{
		"--headless",
		"--nologo",
		"--nodefault",
		"--nolockcheck",
		"--nofirststartwizard",
		"--norestore",
		"-env:UserInstallation=" + fileURL(profileDir),
		"--convert-to",
		job.Format,
		"--outdir",
		stagingOutput,
		stagedInput,
	}
	cmd := exec.CommandContext(runCtx, job.Runtime.SOfficePath, args...)
	configureProcessGroup(cmd)
	cmd.Cancel = func() error { return terminateProcessGroup(cmd.Process) }
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = stagingRoot
	cmd.Env = officeProcessEnvironment(w.environment)
	var commandOutput cappedBuffer
	commandOutput.limit = maxCommandOutputBytes
	cmd.Stdout = &commandOutput
	cmd.Stderr = &commandOutput
	runErr := cmd.Run()
	forceKillProcessGroup(cmd.Process)
	if runCtx.Err() != nil {
		// Preserve the caller's deadline/cancellation. Only the deadline owned
		// by this worker is reported as ErrRenderTimeout.
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return Result{}, fmt.Errorf("%w after %s: %s", ErrRenderTimeout, w.timeout, commandOutput.String())
		}
		return Result{}, runCtx.Err()
	}
	if runErr != nil {
		return Result{}, fmt.Errorf("LibreOffice conversion failed: %w: %s", runErr, commandOutput.String())
	}

	formatExtension := strings.ToLower(strings.SplitN(job.Format, ":", 2)[0])
	inputBase := strings.TrimSuffix(filepath.Base(job.InputPath), filepath.Ext(job.InputPath))
	stagedPath := filepath.Join(stagingOutput, inputBase+"."+formatExtension)
	stagedInfo, err := os.Lstat(stagedPath)
	if err != nil || !stagedInfo.Mode().IsRegular() || stagedInfo.Size() == 0 {
		return Result{}, fmt.Errorf("%w at %s: %s", ErrNoOutput, stagedPath, commandOutput.String())
	}
	if err := commitOutputAtomically(stagedPath, job.OutputPath); err != nil {
		return Result{}, fmt.Errorf("commit office render output: %w", err)
	}
	return Result{
		OutputPath:      job.OutputPath,
		RuntimeRevision: job.Runtime.Revision,
		Duration:        time.Since(started),
	}, nil
}

func (w *OfficeRenderWorker) acquire(ctx context.Context) error {
	select {
	case w.semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *OfficeRenderWorker) release() {
	<-w.semaphore
}

func (w *OfficeRenderWorker) ensureRuntimeAttested(ctx context.Context, runtimeSpec Runtime) error {
	revision := strings.TrimSpace(runtimeSpec.Revision)
	fingerprint := runtimeFingerprint(runtimeSpec)
	w.attestationMu.Lock()
	if cached, ok := w.attestedRevisions[revision]; ok {
		w.attestationMu.Unlock()
		if cached != fingerprint {
			return fmt.Errorf("%w: revision %q changed identity", ErrRuntimeAttestation, revision)
		}
		return nil
	}
	if existing := w.attestationFlights[revision]; existing != nil {
		if existing.fingerprint != fingerprint {
			w.attestationMu.Unlock()
			return fmt.Errorf("%w: revision %q has conflicting runtime paths", ErrRuntimeAttestation, revision)
		}
		w.attestationMu.Unlock()
		select {
		case <-existing.done:
			return existing.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	flight := &attestationFlight{done: make(chan struct{}), fingerprint: fingerprint}
	w.attestationFlights[revision] = flight
	w.attestationMu.Unlock()

	err := defaultRuntimeAttestation(runtimeSpec)
	if err == nil && w.attest != nil {
		err = w.attest(ctx, runtimeSpec)
	}
	if err != nil && !errors.Is(err, ErrRuntimeAttestation) {
		err = fmt.Errorf("%w: %v", ErrRuntimeAttestation, err)
	}
	w.attestationMu.Lock()
	flight.err = err
	if err == nil {
		w.attestedRevisions[revision] = fingerprint
	}
	close(flight.done)
	delete(w.attestationFlights, revision)
	w.attestationMu.Unlock()
	return err
}

func normalizeJob(job Job) (Job, os.FileInfo, error) {
	job.Runtime.Revision = strings.TrimSpace(job.Runtime.Revision)
	job.Runtime.Root = strings.TrimSpace(job.Runtime.Root)
	job.Runtime.SOfficePath = strings.TrimSpace(job.Runtime.SOfficePath)
	job.InputPath = strings.TrimSpace(job.InputPath)
	job.OutputPath = strings.TrimSpace(job.OutputPath)
	job.Format = strings.TrimSpace(job.Format)
	if job.Runtime.Revision == "" || job.Runtime.Root == "" || job.Runtime.SOfficePath == "" {
		return Job{}, nil, fmt.Errorf("%w: runtime revision, root, and soffice path are required", ErrInvalidJob)
	}
	if job.InputPath == "" || job.OutputPath == "" {
		return Job{}, nil, fmt.Errorf("%w: input and output paths are required", ErrInvalidJob)
	}
	var err error
	job.Runtime.Root, err = filepath.Abs(job.Runtime.Root)
	if err != nil {
		return Job{}, nil, fmt.Errorf("%w: invalid runtime root: %v", ErrInvalidJob, err)
	}
	job.Runtime.SOfficePath, err = filepath.Abs(job.Runtime.SOfficePath)
	if err != nil {
		return Job{}, nil, fmt.Errorf("%w: invalid soffice path: %v", ErrInvalidJob, err)
	}
	job.InputPath, err = filepath.Abs(job.InputPath)
	if err != nil {
		return Job{}, nil, fmt.Errorf("%w: invalid input path: %v", ErrInvalidJob, err)
	}
	job.OutputPath, err = filepath.Abs(job.OutputPath)
	if err != nil {
		return Job{}, nil, fmt.Errorf("%w: invalid output path: %v", ErrInvalidJob, err)
	}
	if job.InputPath == job.OutputPath {
		return Job{}, nil, fmt.Errorf("%w: input and output paths must differ", ErrInvalidJob)
	}
	inputInfo, err := os.Stat(job.InputPath)
	if err != nil || !inputInfo.Mode().IsRegular() {
		return Job{}, nil, fmt.Errorf("%w: input must be a readable regular file: %s", ErrInvalidJob, job.InputPath)
	}
	if job.Format == "" {
		job.Format = strings.TrimPrefix(strings.ToLower(filepath.Ext(job.OutputPath)), ".")
	}
	if !validFormat(job.Format) {
		return Job{}, nil, fmt.Errorf("%w: unsupported conversion format %q", ErrInvalidJob, job.Format)
	}
	formatExtension := strings.ToLower(strings.SplitN(job.Format, ":", 2)[0])
	outputExtension := strings.TrimPrefix(strings.ToLower(filepath.Ext(job.OutputPath)), ".")
	if outputExtension != formatExtension {
		return Job{}, nil, fmt.Errorf(
			"%w: output extension %q does not match conversion format %q",
			ErrInvalidJob,
			outputExtension,
			formatExtension,
		)
	}
	return job, inputInfo, nil
}

func validFormat(format string) bool {
	if len(format) == 0 || len(format) > 128 {
		return false
	}
	for index, r := range format {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		if index > 0 && strings.ContainsRune("._:+-", r) {
			continue
		}
		return false
	}
	return true
}

func defaultRuntimeAttestation(runtimeSpec Runtime) error {
	if strings.TrimSpace(runtimeSpec.Revision) == "" {
		return fmt.Errorf("%w: revision is required", ErrRuntimeAttestation)
	}
	root, err := filepath.EvalSymlinks(runtimeSpec.Root)
	if err != nil {
		return fmt.Errorf("%w: resolve runtime root: %v", ErrRuntimeAttestation, err)
	}
	soffice, err := filepath.EvalSymlinks(runtimeSpec.SOfficePath)
	if err != nil {
		return fmt.Errorf("%w: resolve soffice: %v", ErrRuntimeAttestation, err)
	}
	inside, err := pathInside(soffice, root)
	if err != nil || !inside {
		return fmt.Errorf("%w: soffice escapes the managed runtime root", ErrRuntimeAttestation)
	}
	info, err := os.Stat(soffice)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: soffice is not a regular file", ErrRuntimeAttestation)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%w: soffice is not executable", ErrRuntimeAttestation)
	}
	return nil
}

func pathInside(path, root string) (bool, error) {
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	return relativePath == "." || (relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) && !filepath.IsAbs(relativePath)), nil
}

func runtimeFingerprint(runtimeSpec Runtime) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		filepath.Clean(runtimeSpec.Root),
		filepath.Clean(runtimeSpec.SOfficePath),
		strings.TrimSpace(runtimeSpec.Attestation),
	}, "\x00")))
	return hex.EncodeToString(hash[:])
}

func renderKey(job Job, inputInfo os.FileInfo) string {
	payload := strings.Join([]string{
		job.Runtime.Revision,
		job.Runtime.Root,
		job.Runtime.SOfficePath,
		job.Runtime.Attestation,
		job.InputPath,
		fmt.Sprintf("%d", inputInfo.Size()),
		fmt.Sprintf("%d", inputInfo.ModTime().UnixNano()),
		job.OutputPath,
		job.Format,
	}, "\x00")
	hash := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(hash[:])
}

func fileURL(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	slashed := filepath.ToSlash(absolute)
	if runtime.GOOS == "windows" && len(slashed) >= 2 && slashed[1] == ':' {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}

func stageInputFile(source, stagingRoot string) (string, error) {
	inputDirectory := filepath.Join(stagingRoot, "input")
	if err := os.Mkdir(inputDirectory, 0o700); err != nil {
		return "", err
	}
	destination := filepath.Join(inputDirectory, filepath.Base(source))
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return "", err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	return destination, nil
}

func officeProcessEnvironment(overrides []string) []string {
	allowed := []string{
		"APPDATA", "COMSPEC", "HOME", "LANG", "LC_ALL", "LC_CTYPE", "LOCALAPPDATA",
		"PATH", "PATHEXT", "SystemRoot", "TEMP", "TMP", "TMPDIR", "USERPROFILE", "WINDIR",
		"XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "__CF_USER_TEXT_ENCODING",
	}
	values := make(map[string]string, len(allowed)+len(overrides)+1)
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	for _, entry := range overrides {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.TrimSpace(key) != "" {
			values[key] = value
		}
	}
	if runtime.GOOS != "windows" {
		values["SAL_USE_VCLPLUGIN"] = "svp"
	}
	// LibreOffice ships an embedded Python runtime. Without this guard it writes
	// __pycache__ into LibreOfficePython.framework on first conversion, which
	// mutates the sealed app bundle and makes every later strict attestation
	// fail. Runtime revisions are immutable; bytecode stays in memory instead.
	values["PYTHONDONTWRITEBYTECODE"] = "1"
	environment := make([]string, 0, len(values))
	for key, value := range values {
		environment = append(environment, key+"="+value)
	}
	return environment
}

func commitOutputAtomically(stagedPath, outputPath string) error {
	file, err := os.Open(stagedPath)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := replaceFileAtomically(stagedPath, outputPath); err != nil {
		return err
	}
	if directory, err := os.Open(filepath.Dir(outputPath)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	info, err := os.Lstat(outputPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("atomic output verification failed for %s", outputPath)
	}
	return nil
}

type cappedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *cappedBuffer) Write(payload []byte) (int, error) {
	originalLength := len(payload)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if len(payload) > remaining {
			payload = payload[:remaining]
		}
		_, _ = b.buffer.Write(payload)
	}
	return originalLength, nil
}

func (b *cappedBuffer) String() string {
	return strings.TrimSpace(b.buffer.String())
}
