package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"awaken/src/tool/sandbox"
)

type ExecutableObservation struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Version     string `json:"version,omitempty"`
	Provenance  string `json:"provenance"`
	PathIndex   int    `json:"path_index,omitempty"`
	Warning     string `json:"warning,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type WorkspaceEnvironmentObservation struct {
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Evidence string `json:"evidence"`
}

// SkillRootObservation records discovery metadata only. Environment probing
// never reads SKILL.md, executes scripts, or changes Skill registry state.
type SkillRootObservation struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type ExecutionEnvironmentSnapshot struct {
	SnapshotID         string    `json:"snapshot_id"`
	GeneratedAt        time.Time `json:"generated_at"`
	OS                 string    `json:"os"`
	Architecture       string    `json:"architecture"`
	NativeArchitecture string    `json:"native_architecture,omitempty"`
	Translation        string    `json:"translation"`
	Shell              string    `json:"shell,omitempty"`
	PATH               []string  `json:"path"`
	PATHDigest         string    `json:"path_digest,omitempty"`
	PATHEntryCount     int       `json:"path_entry_count,omitempty"`
	PATHTruncated      bool      `json:"path_truncated,omitempty"`
	PATHSource         string    `json:"path_source"`
	// TransportPATH is the complete host-resolved PATH pinned for in-process
	// tool execution. It is deliberately not serialized into provider context,
	// journals, or receipts; PATHDigest binds it into SnapshotID while PATH,
	// PATHEntryCount, and PATHTruncated provide a bounded diagnostic view.
	TransportPATH            string                            `json:"-"`
	WorkingDirectory         string                            `json:"working_directory,omitempty"`
	WorkspaceRoot            string                            `json:"workspace_root,omitempty"`
	SkillRoots               []SkillRootObservation            `json:"skill_roots,omitempty"`
	TemporaryWorkRoot        string                            `json:"temporary_work_root,omitempty"`
	RefreshReason            string                            `json:"refresh_reason"`
	Executables              []ExecutableObservation           `json:"executables,omitempty"`
	AvailablePackageManagers []string                          `json:"available_package_managers,omitempty"`
	EnvironmentReferences    []string                          `json:"environment_references,omitempty"`
	ProxyReferences          []string                          `json:"proxy_references,omitempty"`
	AuthenticationReferences []string                          `json:"authentication_references,omitempty"`
	RuntimeReferences        []string                          `json:"runtime_references,omitempty"`
	WorkspaceEnvironments    []WorkspaceEnvironmentObservation `json:"workspace_environments,omitempty"`
	Confidence               string                            `json:"confidence"`
	Warnings                 []string                          `json:"warnings,omitempty"`
}

type VersionProbeFunc func(ctx context.Context, executable string, maxOutputBytes int) (string, error)
type LookPathFunc func(name string) (string, error)

const maxEnvironmentSnapshotPATHEntries = 128

type ExecutionEnvironmentProbeOptions struct {
	WorkspaceRoot    string
	WorkingDirectory string
	PATH             string
	PATHSource       string
	// PATHWarnings carries warnings from a host-owned PATH resolver when PATH
	// and PATHSource have already been resolved before this probe. Callers must
	// not populate it from model input. Keeping the warnings on the snapshot
	// preserves the provenance of the exact PATH later used by process tools.
	PATHWarnings      []string
	Shell             string
	SkillRoots        []SkillRootObservation
	TemporaryWorkRoot string
	RefreshReason     string
	// EnableSafeLoginShellPATHProbe opts into bounded static parsing of
	// literal PATH assignments. It never starts a shell or sources rc files.
	EnableSafeLoginShellPATHProbe bool
	LoginShellHome                string
	LoginShellConfigPaths         []string
	LoginShellProbeTimeout        time.Duration
	LoginShellMaxBytes            int
	ReadLoginShellFile            LoginShellFileReader
	Executables                   []string
	PackageManagers               []string
	ProbeVersions                 bool
	ProbeTimeout                  time.Duration
	MaxOutputBytes                int
	LookPath                      LookPathFunc
	ProbeVersion                  VersionProbeFunc
	Now                           func() time.Time
}

var defaultEnvironmentExecutables = []string{
	"sh", "bash", "zsh", "pwsh", "powershell", "python3", "python", "node",
	"git", "gh", "go", "java", "soffice", "libreoffice", "pdftoppm", "pdfinfo", "rg", "ffmpeg",
	"pandoc", "docker", "dws", "lark-cli",
}

var defaultPackageManagers = []string{"uv", "pip", "pip3", "npm", "pnpm", "yarn", "bun", "brew", "winget", "choco", "apt"}

// ProbeExecutionEnvironment performs only bounded, read-only probes. It never
// sources shell rc files and never records environment values that may contain
// credentials. Probe failures are localized as warnings.
func ProbeExecutionEnvironment(ctx context.Context, options ExecutionEnvironmentProbeOptions) ExecutionEnvironmentSnapshot {
	if ctx == nil {
		ctx = context.Background()
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	shell := strings.TrimSpace(options.Shell)
	if shell == "" {
		shell = strings.TrimSpace(os.Getenv("SHELL"))
	}
	workingDirectory := strings.TrimSpace(options.WorkingDirectory)
	if workingDirectory == "" {
		workingDirectory, _ = os.Getwd()
	}
	pathValue, pathSource, pathWarnings := resolveExecutionEnvironmentPATH(ctx, options, shell)
	pathWarnings = append(pathWarnings, options.PATHWarnings...)
	transportPATHEntries := boundedPathEntries(pathValue, 0)
	transportPATH := strings.Join(transportPATHEntries, string(os.PathListSeparator))
	pathPreview := append([]string(nil), transportPATHEntries...)
	if len(pathPreview) > maxEnvironmentSnapshotPATHEntries {
		pathPreview = pathPreview[:maxEnvironmentSnapshotPATHEntries]
	}
	pathDigest := sha256.Sum256([]byte(transportPATH))
	skillRoots, skillRootWarnings := normalizeEnvironmentSkillRoots(options.SkillRoots)
	temporaryWorkRoot, temporaryWarnings := normalizeEnvironmentAbsoluteRoot(options.TemporaryWorkRoot, "temporary work root")
	refreshReason := sanitizeProbeText(options.RefreshReason, 256)
	if refreshReason == "" {
		refreshReason = "unspecified"
	}

	snapshot := ExecutionEnvironmentSnapshot{
		GeneratedAt: now().UTC(), OS: runtime.GOOS, Architecture: runtime.GOARCH,
		Shell: shell, PATH: pathPreview,
		PATHDigest: "sha256:" + hex.EncodeToString(pathDigest[:]), PATHEntryCount: len(transportPATHEntries),
		PATHTruncated: len(transportPATHEntries) > len(pathPreview), PATHSource: pathSource, TransportPATH: transportPATH,
		WorkingDirectory: workingDirectory, WorkspaceRoot: strings.TrimSpace(options.WorkspaceRoot),
		SkillRoots: skillRoots, TemporaryWorkRoot: temporaryWorkRoot, RefreshReason: refreshReason,
		Confidence: "observed", Warnings: append(append(pathWarnings, skillRootWarnings...), temporaryWarnings...),
	}
	snapshot.NativeArchitecture, snapshot.Translation = observePlatformArchitecture(ctx)
	snapshot.ProxyReferences, snapshot.AuthenticationReferences, snapshot.RuntimeReferences = presentEnvironmentReferenceNames()
	snapshot.EnvironmentReferences = uniqueProviderStrings(append(append(
		append([]string(nil), snapshot.ProxyReferences...), snapshot.AuthenticationReferences...), snapshot.RuntimeReferences...))
	workspaceEnvironments, workspaceWarnings := discoverWorkspaceEnvironments(snapshot.WorkspaceRoot)
	snapshot.WorkspaceEnvironments = workspaceEnvironments
	snapshot.Warnings = append(snapshot.Warnings, workspaceWarnings...)

	executables := options.Executables
	if executables == nil {
		executables = defaultEnvironmentExecutables
	}
	packageManagers := options.PackageManagers
	if packageManagers == nil {
		packageManagers = defaultPackageManagers
	}
	allNames := uniqueEnvironmentNames(append(append([]string(nil), executables...), packageManagers...))
	lookPath := options.LookPath
	if lookPath == nil {
		lookPath = func(name string) (string, error) {
			return lookPathUsingPATH(name, pathValue)
		}
	}
	probeVersion := options.ProbeVersion
	if probeVersion == nil {
		probePATH := snapshot.PinnedTransportPATHEntries()
		probeVersion = func(ctx context.Context, executable string, maxOutputBytes int) (string, error) {
			return cachedBoundedVersionProbeWithPATH(ctx, executable, maxOutputBytes, probePATH)
		}
	}
	probeTimeout := options.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = 750 * time.Millisecond
	}
	maxOutput := options.MaxOutputBytes
	if maxOutput <= 0 || maxOutput > 16*1024 {
		maxOutput = 4096
	}
	packageManagerSet := make(map[string]bool, len(packageManagers))
	for _, name := range packageManagers {
		packageManagerSet[strings.ToLower(strings.TrimSpace(name))] = true
	}

	for _, name := range allNames {
		if err := ctx.Err(); err != nil {
			snapshot.Warnings = append(snapshot.Warnings, "environment probe cancelled: "+err.Error())
			break
		}
		resolved, err := lookPath(name)
		if err != nil {
			continue
		}
		if absolute, absErr := filepath.Abs(resolved); absErr == nil {
			resolved = absolute
		}
		resolved = filepath.Clean(resolved)
		pathIndex := pathEntryIndex(snapshot.PATH, resolved)
		resolved, provenance := resolveObservedExecutable(name, resolved)
		observation := ExecutableObservation{
			Name: name, Path: resolved, Provenance: provenance, PathIndex: pathIndex,
		}
		fingerprintRequired := options.ProbeVersions || packageManagerSet[strings.ToLower(name)] || strings.EqualFold(name, "git")
		if fingerprintRequired {
			if fingerprint, fingerprintErr := FingerprintExecutable(resolved); fingerprintErr == nil {
				observation.Fingerprint = fingerprint
			} else {
				observation.Warning = sanitizeProbeText(firstNonBlank(observation.Warning, "executable fingerprint unavailable: "+fingerprintErr.Error()), 512)
			}
		}
		if options.ProbeVersions {
			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			version, probeErr := probeVersion(probeCtx, resolved, maxOutput)
			cancel()
			if probeErr != nil {
				observation.Warning = sanitizeProbeText(probeErr.Error(), 512)
			} else {
				observation.Version = sanitizeProbeText(version, 512)
			}
		}
		snapshot.Executables = append(snapshot.Executables, observation)
		if packageManagerSet[strings.ToLower(name)] {
			snapshot.AvailablePackageManagers = append(snapshot.AvailablePackageManagers, name)
		}
	}
	sort.Slice(snapshot.Executables, func(i, j int) bool { return snapshot.Executables[i].Name < snapshot.Executables[j].Name })
	sort.Strings(snapshot.AvailablePackageManagers)
	snapshot.Warnings = uniqueProviderStrings(snapshot.Warnings)
	snapshot.SnapshotID = environmentSnapshotIdentity(snapshot)
	return snapshot
}

func resolveObservedExecutable(name, lookupPath string) (string, string) {
	lookupPath = filepath.Clean(lookupPath)
	if runtime.GOOS == "darwin" && strings.EqualFold(strings.TrimSpace(name), "git") {
		if developerGit := resolveDarwinDeveloperGit(); developerGit != "" {
			return developerGit, "host_trusted_developer_tool"
		}
	}
	if realPath, err := filepath.EvalSymlinks(lookupPath); err == nil {
		if absolute, absErr := filepath.Abs(realPath); absErr == nil {
			realPath = absolute
		}
		return filepath.Clean(realPath), "path_lookup_resolved"
	}
	return lookupPath, "path_lookup"
}

func resolveDarwinDeveloperGit() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	var candidates []string
	if selectedDeveloperDir, err := filepath.EvalSymlinks("/var/db/xcode_select_link"); err == nil && filepath.IsAbs(selectedDeveloperDir) {
		candidates = append(candidates, filepath.Join(selectedDeveloperDir, "usr", "bin", "git"))
	}
	candidates = append(candidates,
		"/Applications/Xcode.app/Contents/Developer/usr/bin/git",
		"/Library/Developer/CommandLineTools/usr/bin/git",
	)
	for _, candidate := range candidates {
		resolved, err := filepath.EvalSymlinks(filepath.Clean(candidate))
		if err != nil || !filepath.IsAbs(resolved) {
			continue
		}
		if !isTrustedDarwinDeveloperGitExecutable(resolved) {
			continue
		}
		return filepath.Clean(resolved)
	}
	return ""
}

func isTrustedDarwinDeveloperGitExecutable(path string) bool {
	if runtime.GOOS != "darwin" || !isAllowedDarwinDeveloperGitPath(path) {
		return false
	}
	info, err := os.Lstat(filepath.Clean(path))
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() &&
		info.Size() > 0 && info.Size() <= maxExecutableFingerprintBytes && info.Mode().Perm()&0o111 != 0 &&
		info.Mode().Perm()&0o022 == 0 && isSystemOwnedExecutable(info)
}

func isAllowedDarwinDeveloperGitPath(path string) bool {
	path = filepath.Clean(path)
	if sameEnvironmentPath(path, "/Library/Developer/CommandLineTools/usr/bin/git") {
		return true
	}
	relative, err := filepath.Rel("/Applications", path)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return false
	}
	parts := strings.Split(relative, string(os.PathSeparator))
	if len(parts) != 6 {
		return false
	}
	appName := strings.ToLower(parts[0])
	return strings.HasPrefix(appName, "xcode") && strings.HasSuffix(appName, ".app") &&
		parts[1] == "Contents" && parts[2] == "Developer" && parts[3] == "usr" && parts[4] == "bin" && parts[5] == "git"
}

const maxExecutableFingerprintBytes int64 = 256 << 20

// FingerprintExecutable hashes the resolved regular file bytes. Candidate
// install plans retain this digest and the typed executor recomputes it just
// before transport, closing path/symlink replacement TOCTOU for the manager
// entrypoint itself.
func FingerprintExecutable(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxExecutableFingerprintBytes {
		return "", fmt.Errorf("executable is not a bounded regular file")
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maxExecutableFingerprintBytes+1))
	if err != nil {
		return "", err
	}
	if written != info.Size() {
		return "", fmt.Errorf("executable changed while fingerprinting")
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func lookPathUsingPATH(name, pathValue string) (string, error) {
	if strings.ContainsAny(name, `/\\`) {
		if info, err := os.Stat(name); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return name, nil
		}
		return "", exec.ErrNotFound
	}
	extensions := []string{""}
	if runtime.GOOS == "windows" {
		extensions = filepath.SplitList(strings.ReplaceAll(os.Getenv("PATHEXT"), ";", string(os.PathListSeparator)))
		if len(extensions) == 0 {
			extensions = []string{".COM", ".EXE", ".BAT", ".CMD"}
		}
	}
	for _, directory := range filepath.SplitList(pathValue) {
		if strings.TrimSpace(directory) == "" {
			continue
		}
		for _, extension := range extensions {
			candidate := filepath.Join(directory, name)
			if extension != "" && !strings.HasSuffix(strings.ToLower(candidate), strings.ToLower(extension)) {
				candidate += extension
			}
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				continue
			}
			if runtime.GOOS == "windows" || info.Mode()&0o111 != 0 {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func environmentSnapshotIdentity(snapshot ExecutionEnvironmentSnapshot) string {
	copySnapshot := snapshot
	copySnapshot.SnapshotID = ""
	data, _ := json.Marshal(copySnapshot)
	digest := sha256.Sum256(data)
	return "env_" + hex.EncodeToString(digest[:12])
}

func boundedPathEntries(value string, limit int) []string {
	parts := filepath.SplitList(value)
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || strings.ContainsRune(part, '\x00') {
			continue
		}
		part = filepath.Clean(part)
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func pathEntryIndex(pathEntries []string, executable string) int {
	directory := filepath.Dir(executable)
	for index, entry := range pathEntries {
		if sameEnvironmentPath(directory, entry) {
			return index
		}
	}
	return -1
}

func sameEnvironmentPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func uniqueEnvironmentNames(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.ContainsAny(value, `/\\`) {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func presentEnvironmentReferenceNames() (proxy []string, authentication []string, runtimeRefs []string) {
	collect := func(keys []string) []string {
		var present []string
		for _, key := range keys {
			if _, exists := os.LookupEnv(key); exists {
				present = append(present, key)
			}
		}
		return present
	}
	proxy = collect([]string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY"})
	authentication = collect([]string{
		"SSH_AUTH_SOCK", "SSH_AGENT_PID", "GIT_ASKPASS", "AWS_PROFILE",
		"GOOGLE_APPLICATION_CREDENTIALS", "AZURE_CONFIG_DIR", "DOCKER_CONFIG",
	})
	runtimeRefs = collect([]string{
		"DOCKER_HOST", "VIRTUAL_ENV", "CONDA_PREFIX", "NVM_BIN", "VOLTA_HOME", "MISE_DATA_DIR",
	})
	return proxy, authentication, runtimeRefs
}

func observePlatformArchitecture(ctx context.Context) (native string, translation string) {
	native = runtime.GOARCH
	translation = "native"
	if runtime.GOOS == "windows" {
		if value := strings.TrimSpace(os.Getenv("PROCESSOR_ARCHITEW6432")); value != "" {
			return strings.ToLower(value), "wow64"
		}
		return native, translation
	}
	if runtime.GOOS != "darwin" {
		return native, translation
	}
	probeCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	output, err := exec.CommandContext(probeCtx, "/usr/sbin/sysctl", "-in", "sysctl.proc_translated").Output()
	if err == nil && strings.TrimSpace(string(output)) == "1" {
		return "arm64", "rosetta"
	}
	return native, translation
}

func discoverWorkspaceEnvironments(root string) ([]WorkspaceEnvironmentObservation, []string) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	type candidate struct{ path, kind, evidence string }
	candidates := []candidate{
		{".venv", "python_virtualenv", "directory"}, {"venv", "python_virtualenv", "directory"},
		{"node_modules", "node_dependencies", "directory"}, {"pyproject.toml", "python_project", "file"},
		{"requirements.txt", "python_requirements", "file"}, {"package.json", "node_project", "file"},
		{"package-lock.json", "npm_lock", "file"}, {"pnpm-lock.yaml", "pnpm_lock", "file"},
		{"yarn.lock", "yarn_lock", "file"}, {"uv.lock", "uv_lock", "file"},
		{".python-version", "python_version", "file"}, {".node-version", "node_version", "file"},
		{".tool-versions", "runtime_versions", "file"},
	}
	observations := make([]WorkspaceEnvironmentObservation, 0, len(candidates))
	var warnings []string
	for _, item := range candidates {
		path := filepath.Join(root, item.path)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			warnings = append(warnings, "ignored symlinked workspace environment marker: "+item.path)
			continue
		}
		if (item.evidence == "directory") != info.IsDir() {
			continue
		}
		observations = append(observations, WorkspaceEnvironmentObservation{Kind: item.kind, Path: item.path, Evidence: item.evidence})
	}
	return observations, warnings
}

type boundedProbeWriter struct {
	mu        sync.Mutex
	remaining int
	data      []byte
}

type versionProbeCacheEntry struct {
	value     string
	errorText string
	expiresAt time.Time
}

type versionProbePolicy struct {
	trustedRoots []string
	pathEntries  []string
}

var versionProbeCache sync.Map

// cachedBoundedVersionProbe avoids re-spawning the same trusted system CLI for
// every run. The executable is fingerprinted before cache lookup, so replacing
// bytes cannot reuse an earlier version observation.
func cachedBoundedVersionProbe(ctx context.Context, executable string, maxOutputBytes int) (string, error) {
	return cachedBoundedVersionProbeWithPATH(ctx, executable, maxOutputBytes, nil)
}

func cachedBoundedVersionProbeWithPATH(ctx context.Context, executable string, maxOutputBytes int, pathEntries []string) (string, error) {
	policy := versionProbePolicy{trustedRoots: trustedVersionProbeRootsForExecutable(executable), pathEntries: append([]string(nil), pathEntries...)}
	if err := requireFullVersionProbeSandbox(); err != nil {
		return "", err
	}
	canonical, fingerprint, err := validateVersionProbeExecutable(executable, policy.trustedRoots)
	if err != nil {
		return "", err
	}
	cacheKey := strings.Join([]string{
		"version_probe_v2", canonical, fingerprint, fmt.Sprintf("%d", maxOutputBytes),
		strings.Join(minimalVersionProbePATH(policy.pathEntries, policy.trustedRoots), string(os.PathListSeparator)),
	}, "|")
	now := time.Now()
	if cached, ok := versionProbeCache.Load(cacheKey); ok {
		entry := cached.(versionProbeCacheEntry)
		if now.Before(entry.expiresAt) {
			if entry.errorText != "" {
				return entry.value, errors.New(entry.errorText)
			}
			return entry.value, nil
		}
		versionProbeCache.Delete(cacheKey)
	}
	value, err := boundedVersionProbeWithPolicy(ctx, canonical, maxOutputBytes, policy)
	entry := versionProbeCacheEntry{value: value, expiresAt: now.Add(30 * time.Second)}
	if err != nil {
		entry.errorText = err.Error()
	}
	versionProbeCache.Store(cacheKey, entry)
	return value, err
}

func (w *boundedProbeWriter) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	original := len(value)
	if w.remaining <= 0 {
		return original, nil
	}
	if len(value) > w.remaining {
		value = value[:w.remaining]
	}
	w.data = append(w.data, value...)
	w.remaining -= len(value)
	return original, nil
}

func boundedVersionProbe(ctx context.Context, executable string, maxOutputBytes int) (string, error) {
	return boundedVersionProbeWithPolicy(ctx, executable, maxOutputBytes, versionProbePolicy{
		trustedRoots: trustedVersionProbeRootsForExecutable(executable),
	})
}

func trustedVersionProbeRootsForExecutable(executable string) []string {
	roots := defaultTrustedVersionProbeRoots()
	if isTrustedDarwinDeveloperGitExecutable(executable) {
		roots = append(roots, filepath.Dir(filepath.Clean(executable)))
	}
	return roots
}

func defaultTrustedVersionProbeRoots() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/bin", "/sbin", "/usr/bin", "/usr/sbin",
			"/System/Library", "/Library/Apple/usr/bin",
		}
	case "linux":
		return []string{"/bin", "/sbin", "/usr/bin", "/usr/sbin"}
	default:
		// Windows currently has no SandboxFull implementation. Keep the root
		// set empty so a future adapter cannot silently widen this contract.
		return nil
	}
}

func requireFullVersionProbeSandbox() error {
	supervisor := sandbox.DefaultAdapter()
	if supervisor == nil || supervisor.SupportLevel() != sandbox.SandboxFull {
		return fmt.Errorf("%w: executable version probe requires a read-only full sandbox", ErrExecutionSandboxRequired)
	}
	return nil
}

func validateVersionProbeExecutable(executable string, trustedRoots []string) (string, string, error) {
	canonical, ok := AbsoluteCleanPath(executable, "")
	if !ok {
		return "", "", errors.New("version probe skipped: executable path is not absolute")
	}
	info, err := os.Lstat(canonical)
	if err != nil {
		return "", "", fmt.Errorf("version probe skipped: executable metadata unavailable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", "", errors.New("version probe skipped: executable path is a symlink")
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxExecutableFingerprintBytes {
		return "", "", errors.New("version probe skipped: executable is not a bounded regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", "", errors.New("version probe skipped: regular file is not executable")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return "", "", errors.New("version probe skipped: executable is group/world writable")
	}
	realPath, err := filepath.EvalSymlinks(canonical)
	if err != nil {
		return "", "", fmt.Errorf("version probe skipped: executable real path unavailable: %w", err)
	}
	realPath, ok = AbsoluteCleanPath(realPath, "")
	if !ok || !sameEnvironmentPath(canonical, realPath) {
		return "", "", errors.New("version probe skipped: executable path contains a replaceable symlink")
	}
	if !pathWithinVersionProbeRoots(realPath, trustedRoots) {
		return "", "", errors.New("version probe skipped: executable is outside host-trusted version-probe roots")
	}
	fingerprint, err := FingerprintExecutable(realPath)
	if err != nil {
		return "", "", fmt.Errorf("version probe skipped: executable fingerprint unavailable: %w", err)
	}
	return realPath, fingerprint, nil
}

func pathWithinVersionProbeRoots(path string, trustedRoots []string) bool {
	for _, root := range trustedRoots {
		root, ok := AbsoluteCleanPath(root, "")
		if !ok {
			continue
		}
		if realRoot, err := filepath.EvalSymlinks(root); err == nil {
			root = filepath.Clean(realRoot)
		}
		if PathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func minimalVersionProbePATH(pathEntries, trustedRoots []string) []string {
	result := make([]string, 0, len(pathEntries))
	seen := map[string]bool{}
	for _, entry := range pathEntries {
		entry, ok := AbsoluteCleanPath(entry, "")
		if !ok {
			continue
		}
		if realEntry, err := filepath.EvalSymlinks(entry); err == nil {
			entry = filepath.Clean(realEntry)
		}
		info, err := os.Stat(entry)
		if err != nil || !info.IsDir() || !pathWithinVersionProbeRoots(entry, trustedRoots) || seen[entry] {
			continue
		}
		seen[entry] = true
		result = append(result, entry)
	}
	return result
}

func boundedVersionProbeWithPolicy(ctx context.Context, executable string, maxOutputBytes int, policy versionProbePolicy) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := requireFullVersionProbeSandbox(); err != nil {
		return "", err
	}
	canonical, fingerprint, err := validateVersionProbeExecutable(executable, policy.trustedRoots)
	if err != nil {
		return "", err
	}
	if maxOutputBytes <= 0 || maxOutputBytes > 16*1024 {
		maxOutputBytes = 4096
	}
	temporaryHome, err := os.MkdirTemp("", "awaken-version-probe-")
	if err != nil {
		return "", fmt.Errorf("version probe could not create an empty temporary home: %w", err)
	}
	defer os.RemoveAll(temporaryHome)

	cmd := exec.CommandContext(ctx, canonical, "--version")
	cmd.Dir = temporaryHome
	cmd.Env = []string{
		"HOME=" + temporaryHome,
		"TMPDIR=" + temporaryHome,
		"PATH=" + strings.Join(minimalVersionProbePATH(policy.pathEntries, policy.trustedRoots), string(os.PathListSeparator)),
		"LANG=C",
		"LC_ALL=C",
		"NO_COLOR=1",
	}
	supervisor := sandbox.DefaultAdapter()
	result, sandboxErr := supervisor.ApplySandbox(cmd, sandbox.TierInspectReadOnly, temporaryHome)
	if sandboxErr != nil || !result.Applied || result.Support != sandbox.SandboxFull {
		if sandboxErr != nil {
			return "", sandboxErr
		}
		return "", fmt.Errorf("%w: executable version probe sandbox was not applied", ErrExecutionSandboxRequired)
	}
	// ApplySandbox may wrap the command but must not start it. Revalidate the
	// original trusted binary immediately before transport so a replacement
	// cannot silently diverge from the cache/snapshot identity.
	revalidatedPath, revalidatedFingerprint, revalidateErr := validateVersionProbeExecutable(canonical, policy.trustedRoots)
	if revalidateErr != nil || !sameEnvironmentPath(canonical, revalidatedPath) || fingerprint != revalidatedFingerprint {
		if revalidateErr != nil {
			return "", revalidateErr
		}
		return "", errors.New("version probe skipped: executable changed before transport")
	}
	writer := &boundedProbeWriter{remaining: maxOutputBytes}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err = cmd.Run()
	value := strings.TrimSpace(string(writer.data))
	if err != nil && value == "" {
		return "", err
	}
	if value == "" {
		return "", errors.New("version probe produced no output")
	}
	if line, _, found := strings.Cut(value, "\n"); found {
		value = line
	}
	return value, nil
}

func sanitizeProbeText(value string, limit int) string {
	value = strings.TrimSpace(value)
	for _, marker := range []string{"Bearer ", "sk-", "ghp_", "github_pat_", "xoxb-", "xoxp-"} {
		value = redactTokenAfterMarker(value, marker)
	}
	if limit > 0 && len([]rune(value)) > limit {
		value = string([]rune(value)[:limit]) + "…"
	}
	return value
}

func redactTokenAfterMarker(value, marker string) string {
	for {
		index := strings.Index(value, marker)
		if index < 0 {
			return value
		}
		start := index + len(marker)
		end := start
		for end < len(value) {
			char := value[end]
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
				end++
				continue
			}
			break
		}
		if end == start {
			return value
		}
		value = value[:start] + "[REDACTED]" + value[end:]
	}
}

type environmentSnapshotContextKey struct{}

func ContextWithExecutionEnvironmentSnapshot(ctx context.Context, snapshot ExecutionEnvironmentSnapshot) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, environmentSnapshotContextKey{}, snapshot.Clone())
}

func ExecutionEnvironmentSnapshotFromContext(ctx context.Context) (ExecutionEnvironmentSnapshot, bool) {
	if ctx == nil {
		return ExecutionEnvironmentSnapshot{}, false
	}
	snapshot, ok := ctx.Value(environmentSnapshotContextKey{}).(ExecutionEnvironmentSnapshot)
	return snapshot.Clone(), ok && strings.TrimSpace(snapshot.SnapshotID) != ""
}

// Clone returns a deep copy suitable for retaining at a run boundary. Every
// slice is copied so caller mutation cannot rewrite a pinned executable or
// package-manager observation after ToolRuntime construction.
func (s ExecutionEnvironmentSnapshot) Clone() ExecutionEnvironmentSnapshot {
	s.PATH = append([]string(nil), s.PATH...)
	s.Executables = append([]ExecutableObservation(nil), s.Executables...)
	s.AvailablePackageManagers = append([]string(nil), s.AvailablePackageManagers...)
	s.EnvironmentReferences = append([]string(nil), s.EnvironmentReferences...)
	s.ProxyReferences = append([]string(nil), s.ProxyReferences...)
	s.AuthenticationReferences = append([]string(nil), s.AuthenticationReferences...)
	s.RuntimeReferences = append([]string(nil), s.RuntimeReferences...)
	s.WorkspaceEnvironments = append([]WorkspaceEnvironmentObservation(nil), s.WorkspaceEnvironments...)
	s.SkillRoots = append([]SkillRootObservation(nil), s.SkillRoots...)
	s.Warnings = append([]string(nil), s.Warnings...)
	return s
}

// PinnedTransportPATH returns the complete PATH captured for host process
// transport. Snapshots restored from serialized history intentionally lack the
// in-memory value and fall back to their bounded diagnostic preview; replay
// must not start new tool processes from such historical snapshots.
func (s ExecutionEnvironmentSnapshot) PinnedTransportPATH() string {
	if strings.TrimSpace(s.TransportPATH) != "" {
		return s.TransportPATH
	}
	if s.PATHTruncated {
		return ""
	}
	return strings.Join(s.PATH, string(os.PathListSeparator))
}

func (s ExecutionEnvironmentSnapshot) PinnedTransportPATHEntries() []string {
	return filepath.SplitList(s.PinnedTransportPATH())
}

func (s ExecutionEnvironmentSnapshot) Executable(name string) (ExecutableObservation, bool) {
	for _, executable := range s.Executables {
		if strings.EqualFold(executable.Name, strings.TrimSpace(name)) {
			return executable, true
		}
	}
	return ExecutableObservation{}, false
}

// IsTaskControlledPath reports whether path is inside workspace-, temporary-,
// or Skill-owned content. These roots are task/model-influenceable and must not
// supply an executable that receives an open-network sandbox merely because it
// was observed in PATH and fingerprinted.
func (s ExecutionEnvironmentSnapshot) IsTaskControlledPath(path string) bool {
	path, ok := AbsoluteCleanPath(path, "")
	if !ok {
		return true
	}
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		path = filepath.Clean(realPath)
	}
	roots := []string{s.WorkspaceRoot, s.TemporaryWorkRoot}
	for _, skillRoot := range s.SkillRoots {
		roots = append(roots, skillRoot.Path)
	}
	for _, root := range roots {
		root, ok := AbsoluteCleanPath(root, "")
		if !ok {
			continue
		}
		if PathWithinRoot(path, root) {
			return true
		}
		if realRoot, err := filepath.EvalSymlinks(root); err == nil && PathWithinRoot(path, realRoot) {
			return true
		}
	}
	return false
}

func (s ExecutionEnvironmentSnapshot) Validate() error {
	if strings.TrimSpace(s.SnapshotID) == "" {
		return fmt.Errorf("environment snapshot id is required")
	}
	if strings.TrimSpace(s.OS) == "" || strings.TrimSpace(s.Architecture) == "" {
		return fmt.Errorf("environment snapshot OS and architecture are required")
	}
	return nil
}
