package doctor

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	categoryToolchain = "toolchain"
	categoryWorkspace = "workspace"
	categoryConfig    = "config"
	categoryRuntime   = "runtime"
)

var (
	defaultPorts      = []int{5174, 5173}
	providerEnvKeys   = []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL_ID", "LLM_PROTOCOL", "LLM_AUTH_SCHEME"}
	requiredAPIKeyKey = "LLM_API_KEY"
)

// Options controls a doctor run. Probes are injectable so tests never need to
// shell out or depend on the caller's real environment.
type Options struct {
	WorkspaceRoot    string
	ConfigDir        string
	DesktopConfigDir string
	Ports            []int

	Env      EnvProbe
	Files    FileProbe
	Commands CommandProbe
}

// Runner executes the Awaken developer-readiness diagnostics.
type Runner struct {
	workspaceRoot    string
	configDir        string
	desktopConfigDir string
	ports            []int

	env      EnvProbe
	files    FileProbe
	commands CommandProbe
}

// NewRunner creates a runner with production probes where none are provided.
func NewRunner(opts Options) *Runner {
	env := opts.Env
	if env == nil {
		env = osEnvProbe{}
	}
	files := opts.Files
	if files == nil {
		files = osFileProbe{}
	}
	commands := opts.Commands
	if commands == nil {
		commands = LocalCommandProbe{Timeout: 2 * time.Second}
	}

	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot == "" {
		if cwd, err := env.WorkingDir(); err == nil && cwd != "" {
			workspaceRoot = cwd
		} else {
			workspaceRoot = "."
		}
	}

	configDir := strings.TrimSpace(opts.ConfigDir)
	if configDir == "" {
		if value, ok := env.LookupEnv("AWAKEN_CONFIG_DIR"); ok && strings.TrimSpace(value) != "" {
			configDir = strings.TrimSpace(value)
		} else {
			configDir = workspaceRoot
		}
	}

	desktopConfigDir := strings.TrimSpace(opts.DesktopConfigDir)
	if desktopConfigDir == "" {
		if value, ok := env.LookupEnv("AWAKEN_CONFIG_DIR"); ok && strings.TrimSpace(value) != "" {
			desktopConfigDir = strings.TrimSpace(value)
		} else {
			desktopConfigDir = defaultDesktopConfigDir(env)
		}
	}

	ports := opts.Ports
	if len(ports) == 0 {
		ports = defaultPorts
	}

	return &Runner{
		workspaceRoot:    workspaceRoot,
		configDir:        configDir,
		desktopConfigDir: desktopConfigDir,
		ports:            append([]int(nil), ports...),
		env:              env,
		files:            files,
		commands:         commands,
	}
}

// Run executes a doctor report with default option handling.
func Run(ctx context.Context, opts Options) Report {
	return NewRunner(opts).Run(ctx)
}

// Run executes all checks.
func (r *Runner) Run(ctx context.Context) Report {
	checks := []CheckResult{
		r.checkGoVersion(ctx),
		r.checkNodeNPM(ctx),
		r.checkGitWorktree(ctx),
		r.checkProviderConfig(),
		r.checkDesktopConfigPath(),
		r.checkLocalRuntimeHints(ctx),
	}
	return NewReport(checks)
}

func (r *Runner) checkGoVersion(ctx context.Context) CheckResult {
	required := r.requiredGoVersion()
	spec := CommandSpec{Name: "go", Args: []string{"version"}}
	result := r.commands.Run(ctx, spec)
	output := redactCommandOutput(result)

	if !result.OK() {
		return CheckResult{
			ID:             "go-version",
			Name:           "Go version",
			Category:       categoryToolchain,
			Status:         StatusFailure,
			Severity:       SeverityError,
			Summary:        "Go is not available or `go version` failed.",
			Details:        commandFailureDetails(spec, result),
			Remediation:    []string{"Install the Go toolchain required by go.mod, then rerun Awaken checks."},
			RedactedOutput: output,
		}
	}

	installed, ok := parseGoVersionOutput(result.Stdout)
	details := []string{fmt.Sprintf("Installed Go: %s", strings.TrimSpace(result.Stdout))}
	if required != "" {
		details = append(details, "go.mod requires Go "+required)
	}
	if !ok {
		return CheckResult{
			ID:             "go-version",
			Name:           "Go version",
			Category:       categoryToolchain,
			Status:         StatusWarning,
			Severity:       SeverityWarning,
			Summary:        "Go is available, but doctor could not parse its version.",
			Details:        details,
			Remediation:    []string{"Verify the installed Go version satisfies go.mod."},
			RedactedOutput: output,
		}
	}
	if required != "" && compareVersion(installed, required) < 0 {
		return CheckResult{
			ID:             "go-version",
			Name:           "Go version",
			Category:       categoryToolchain,
			Status:         StatusFailure,
			Severity:       SeverityError,
			Summary:        "Installed Go is older than the version required by go.mod.",
			Details:        details,
			Remediation:    []string{"Upgrade Go to " + required + " or newer."},
			RedactedOutput: output,
		}
	}

	return CheckResult{
		ID:             "go-version",
		Name:           "Go version",
		Category:       categoryToolchain,
		Status:         StatusOK,
		Severity:       SeverityInfo,
		Summary:        "Go toolchain is available.",
		Details:        details,
		RedactedOutput: output,
	}
}

func (r *Runner) checkNodeNPM(ctx context.Context) CheckResult {
	nodeSpec := CommandSpec{Name: "node", Args: []string{"--version"}}
	npmSpec := CommandSpec{Name: "npm", Args: []string{"--version"}}
	node := r.commands.Run(ctx, nodeSpec)
	npm := r.commands.Run(ctx, npmSpec)

	details := []string{}
	if node.OK() {
		details = append(details, "node: "+strings.TrimSpace(node.Stdout))
	} else {
		details = append(details, commandFailureDetails(nodeSpec, node)...)
	}
	if npm.OK() {
		details = append(details, "npm: "+strings.TrimSpace(npm.Stdout))
	} else {
		details = append(details, commandFailureDetails(npmSpec, npm)...)
	}

	output := strings.TrimSpace(strings.Join([]string{
		redactCommandOutput(node),
		redactCommandOutput(npm),
	}, "\n"))
	if node.OK() && npm.OK() {
		return CheckResult{
			ID:             "node-npm",
			Name:           "Node/npm availability",
			Category:       categoryToolchain,
			Status:         StatusOK,
			Severity:       SeverityInfo,
			Summary:        "Node.js and npm are available.",
			Details:        details,
			RedactedOutput: output,
		}
	}

	return CheckResult{
		ID:             "node-npm",
		Name:           "Node/npm availability",
		Category:       categoryToolchain,
		Status:         StatusFailure,
		Severity:       SeverityError,
		Summary:        "Node.js or npm is missing.",
		Details:        details,
		Remediation:    []string{"Install Node.js with npm, then run `npm install` from the Awaken repo root."},
		RedactedOutput: output,
	}
}

func (r *Runner) checkGitWorktree(ctx context.Context) CheckResult {
	spec := CommandSpec{Name: "git", Args: []string{"status", "--porcelain"}, Dir: r.workspaceRoot}
	result := r.commands.Run(ctx, spec)
	output := redactCommandOutput(result)
	if !result.OK() {
		return CheckResult{
			ID:             "git-worktree",
			Name:           "Git worktree cleanliness",
			Category:       categoryWorkspace,
			Status:         StatusFailure,
			Severity:       SeverityError,
			Summary:        "Unable to inspect git worktree status.",
			Details:        commandFailureDetails(spec, result),
			Remediation:    []string{"Run `git status --short` in the Awaken repo and resolve the git issue."},
			RedactedOutput: output,
		}
	}

	trimmed := strings.TrimSpace(result.Stdout)
	if trimmed == "" {
		return CheckResult{
			ID:             "git-worktree",
			Name:           "Git worktree cleanliness",
			Category:       categoryWorkspace,
			Status:         StatusOK,
			Severity:       SeverityInfo,
			Summary:        "Git worktree is clean.",
			Details:        []string{"Workspace root: " + r.workspaceRoot},
			RedactedOutput: output,
		}
	}

	return CheckResult{
		ID:             "git-worktree",
		Name:           "Git worktree cleanliness",
		Category:       categoryWorkspace,
		Status:         StatusWarning,
		Severity:       SeverityWarning,
		Summary:        "Git worktree has uncommitted changes.",
		Details:        append([]string{"Workspace root: " + r.workspaceRoot}, limitLines(trimmed, 12)...),
		Remediation:    []string{"Review `git status --short` before merging or publishing this branch."},
		RedactedOutput: output,
	}
}

func (r *Runner) checkProviderConfig() CheckResult {
	detection := r.detectProviderConfig()
	details := []string{"Config directory: " + detection.ConfigDir}
	details = append(details, detection.Details...)

	remediation := []string{}
	if !detection.HasAPIKey {
		remediation = append(remediation, "Set LLM_API_KEY in the environment or in "+filepath.Join(detection.ConfigDir, ".env")+".")
	}
	if !detection.HasModel {
		remediation = append(remediation, "Set llm.model_id in config.toml or LLM_MODEL_ID in the environment.")
	}

	if detection.HasAPIKey && detection.HasModel {
		return CheckResult{
			ID:             "provider-config",
			Name:           "Provider env/config presence",
			Category:       categoryConfig,
			Status:         StatusOK,
			Severity:       SeverityInfo,
			Summary:        "Provider credentials and model config are present.",
			Details:        details,
			RedactedOutput: detection.RedactedOutput,
		}
	}

	return CheckResult{
		ID:             "provider-config",
		Name:           "Provider env/config presence",
		Category:       categoryConfig,
		Status:         StatusWarning,
		Severity:       SeverityWarning,
		Summary:        "Provider configuration is incomplete.",
		Details:        details,
		Remediation:    remediation,
		RedactedOutput: detection.RedactedOutput,
	}
}

func (r *Runner) checkDesktopConfigPath() CheckResult {
	dir := r.desktopConfigDir
	envPath := filepath.Join(dir, ".env")
	configPath := filepath.Join(dir, "config.toml")

	dirExists, dirErr := r.files.Exists(dir)
	envExists, envErr := r.files.Exists(envPath)
	configExists, configErr := r.files.Exists(configPath)

	details := []string{"Desktop config directory: " + dir}
	appendExistenceDetail := func(label string, exists bool, err error) {
		if err != nil {
			details = append(details, fmt.Sprintf("%s: error: %v", label, err))
			return
		}
		if exists {
			details = append(details, label+": exists")
		} else {
			details = append(details, label+": missing")
		}
	}
	appendExistenceDetail("directory", dirExists, dirErr)
	appendExistenceDetail(".env", envExists, envErr)
	appendExistenceDetail("config.toml", configExists, configErr)

	if dirErr == nil && envErr == nil && configErr == nil && dirExists && envExists && configExists {
		return CheckResult{
			ID:       "desktop-config-path",
			Name:     "Desktop runtime config path",
			Category: categoryConfig,
			Status:   StatusOK,
			Severity: SeverityInfo,
			Summary:  "Desktop runtime config directory and files exist.",
			Details:  details,
		}
	}

	return CheckResult{
		ID:          "desktop-config-path",
		Name:        "Desktop runtime config path",
		Category:    categoryConfig,
		Status:      StatusWarning,
		Severity:    SeverityWarning,
		Summary:     "Desktop runtime config directory is not fully initialized.",
		Details:     details,
		Remediation: []string{"Run `npm run open:desktop` once to let Electron bootstrap the desktop config templates."},
	}
}

func (r *Runner) checkLocalRuntimeHints(ctx context.Context) CheckResult {
	details := []string{}
	outputs := []string{}

	for _, port := range r.ports {
		spec := CommandSpec{Name: "lsof", Args: []string{"-nP", "-iTCP:" + strconv.Itoa(port), "-sTCP:LISTEN"}}
		result := r.commands.Run(ctx, spec)
		if result.OK() && strings.TrimSpace(result.Stdout) != "" {
			details = append(details, fmt.Sprintf("Port %d has a local listener.", port))
		} else {
			details = append(details, fmt.Sprintf("Port %d has no listener hint.", port))
		}
		if redacted := redactCommandOutput(result); redacted != "" {
			outputs = append(outputs, spec.Display()+":\n"+redacted)
		}
	}

	processSpec := CommandSpec{Name: "pgrep", Args: []string{"-fl", "awaken-core"}}
	processResult := r.commands.Run(ctx, processSpec)
	if processResult.OK() && strings.TrimSpace(processResult.Stdout) != "" {
		details = append(details, "awaken-core process hint found.")
	} else {
		details = append(details, "No awaken-core process hint found.")
	}
	if redacted := redactCommandOutput(processResult); redacted != "" {
		outputs = append(outputs, processSpec.Display()+":\n"+redacted)
	}

	return CheckResult{
		ID:             "local-runtime-hints",
		Name:           "Local port/process hints",
		Category:       categoryRuntime,
		Status:         StatusInfo,
		Severity:       SeverityInfo,
		Summary:        "Local runtime hints are informational; Awaken does not need to be running for doctor readiness.",
		Details:        details,
		RedactedOutput: strings.TrimSpace(strings.Join(outputs, "\n\n")),
	}
}

type providerConfigDetection struct {
	ConfigDir      string
	HasAPIKey      bool
	HasModel       bool
	HasBaseURL     bool
	Details        []string
	RedactedOutput string
}

func (r *Runner) detectProviderConfig() providerConfigDetection {
	detection := providerConfigDetection{ConfigDir: r.configDir}
	redactedLines := []string{}

	for _, key := range providerEnvKeys {
		value, ok := r.env.LookupEnv(key)
		if ok && strings.TrimSpace(value) != "" {
			switch key {
			case "LLM_API_KEY":
				detection.HasAPIKey = true
				detection.Details = append(detection.Details, "LLM_API_KEY is present in environment.")
			case "LLM_MODEL_ID":
				detection.HasModel = true
				detection.Details = append(detection.Details, "LLM_MODEL_ID is present in environment.")
			case "LLM_BASE_URL":
				detection.HasBaseURL = true
				detection.Details = append(detection.Details, "LLM_BASE_URL is present in environment.")
			}
			redactedLines = append(redactedLines, key+"="+value)
		}
	}

	envPath := filepath.Join(r.configDir, ".env")
	if content, err := r.files.ReadFile(envPath); err == nil {
		detection.Details = append(detection.Details, ".env file exists.")
		if value, ok := parseEnvValue(content, requiredAPIKeyKey); ok && strings.TrimSpace(value) != "" {
			detection.HasAPIKey = true
			detection.Details = append(detection.Details, "LLM_API_KEY is present in .env.")
			redactedLines = append(redactedLines, requiredAPIKeyKey+"="+value)
		} else {
			detection.Details = append(detection.Details, "LLM_API_KEY is missing or empty in .env.")
		}
	} else {
		detection.Details = append(detection.Details, ".env file not found at "+envPath+".")
	}

	configPath := filepath.Join(r.configDir, "config.toml")
	if content, err := r.files.ReadFile(configPath); err == nil {
		detection.Details = append(detection.Details, "config.toml file exists.")
		if value, ok := parseTomlValue(content, "llm", "model_id"); ok && strings.TrimSpace(value) != "" {
			detection.HasModel = true
			detection.Details = append(detection.Details, "llm.model_id is present in config.toml.")
			redactedLines = append(redactedLines, "model_id="+value)
		}
		if value, ok := parseTomlValue(content, "llm", "base_url"); ok && strings.TrimSpace(value) != "" {
			detection.HasBaseURL = true
			detection.Details = append(detection.Details, "llm.base_url is present in config.toml.")
			redactedLines = append(redactedLines, "base_url="+value)
		}
	} else {
		detection.Details = append(detection.Details, "config.toml file not found at "+configPath+".")
	}

	if !detection.HasBaseURL {
		detection.Details = append(detection.Details, "No explicit base URL found; runtime will fall back to its default if no config is provided.")
	}
	detection.RedactedOutput = strings.Join(redactLines(redactedLines), "\n")
	return detection
}

func (r *Runner) requiredGoVersion() string {
	content, err := r.files.ReadFile(filepath.Join(r.workspaceRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
	}
	return ""
}

func commandFailureDetails(spec CommandSpec, result CommandResult) []string {
	details := []string{fmt.Sprintf("Command `%s` exited with code %d.", spec.Display(), result.ExitCode)}
	if result.Err != nil {
		details = append(details, "Error: "+result.Err.Error())
	}
	return details
}

func limitLines(text string, maxLines int) []string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) <= maxLines {
		return redactLines(lines)
	}
	limited := append([]string{}, lines[:maxLines]...)
	limited = append(limited, fmt.Sprintf("... %d more line(s)", len(lines)-maxLines))
	return redactLines(limited)
}

func defaultDesktopConfigDir(env EnvProbe) string {
	home, err := env.HomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(".", "config")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "@awaken", "desktop", "config")
	case "windows":
		if appData, ok := env.LookupEnv("APPDATA"); ok && strings.TrimSpace(appData) != "" {
			return filepath.Join(appData, "@awaken", "desktop", "config")
		}
		return filepath.Join(home, "AppData", "Roaming", "@awaken", "desktop", "config")
	default:
		if xdg, ok := env.LookupEnv("XDG_CONFIG_HOME"); ok && strings.TrimSpace(xdg) != "" {
			return filepath.Join(xdg, "@awaken", "desktop", "config")
		}
		return filepath.Join(home, ".config", "@awaken", "desktop", "config")
	}
}

func parseGoVersionOutput(output string) (string, bool) {
	fields := strings.Fields(output)
	for _, field := range fields {
		if strings.HasPrefix(field, "go1.") {
			return strings.TrimPrefix(field, "go"), true
		}
	}
	return "", false
}

func compareVersion(left, right string) int {
	l := parseVersionParts(left)
	r := parseVersionParts(right)
	for i := 0; i < len(l); i++ {
		if l[i] < r[i] {
			return -1
		}
		if l[i] > r[i] {
			return 1
		}
	}
	return 0
}

func parseVersionParts(version string) [3]int {
	var parts [3]int
	for i, part := range strings.Split(version, ".") {
		if i >= len(parts) {
			break
		}
		part = strings.TrimFunc(part, func(r rune) bool {
			return r < '0' || r > '9'
		})
		if parsed, err := strconv.Atoi(part); err == nil {
			parts[i] = parsed
		}
	}
	return parts
}

func parseEnvValue(content, key string) (string, bool) {
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return trimConfigValue(value), true
	}
	return "", false
}

func parseTomlValue(content, section, key string) (string, bool) {
	currentSection := ""
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(stripInlineComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(strings.Trim(line, "[]"))
			continue
		}
		if currentSection != section {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return trimConfigValue(value), true
	}
	return "", false
}

func trimConfigValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, `"`)
	value = strings.TrimSuffix(value, `"`)
	value = strings.TrimPrefix(value, `'`)
	value = strings.TrimSuffix(value, `'`)
	return value
}

func stripInlineComment(line string) string {
	inSingleQuote := false
	inDoubleQuote := false
	for i, r := range line {
		switch r {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '#':
			if !inSingleQuote && !inDoubleQuote {
				return line[:i]
			}
		}
	}
	return line
}
