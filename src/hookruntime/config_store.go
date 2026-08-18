package hookruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"awaken/src/desktopdata"
)

var hookIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)

type Store struct {
	root       string
	trust      *TrustStore
	mutationMu sync.Mutex
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = desktopdata.AwakenHomeRoot()
	}
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("hookruntime: AWAKEN_HOME must be an absolute path")
	}
	root = filepath.Clean(root)
	return &Store{root: root, trust: NewTrustStore(desktopdata.HookTrustPath(root))}, nil
}

func (s *Store) UserConfigPath() string  { return desktopdata.HookConfigPath(s.root) }
func (s *Store) TrustStore() *TrustStore { return s.trust }

type sourceCandidate struct {
	kind        SourceKind
	path        string
	projectRoot string
}

func (s *Store) BuildSnapshot(projectID string, projectRoots []string, sessionWorkspace bool) (AvailabilitySnapshot, error) {
	return s.buildSnapshot(projectID, projectRoots, sessionWorkspace, false)
}

func (s *Store) buildSnapshot(projectID string, projectRoots []string, sessionWorkspace, includeReview bool) (AvailabilitySnapshot, error) {
	snapshot := EmptySnapshot()
	candidates := []sourceCandidate{{kind: SourceUser, path: s.UserConfigPath()}}
	if strings.TrimSpace(projectID) != "" && !sessionWorkspace {
		seen := map[string]struct{}{}
		for _, root := range projectRoots {
			root = filepath.Clean(strings.TrimSpace(root))
			if root == "." || !filepath.IsAbs(root) {
				continue
			}
			if _, ok := seen[root]; ok {
				continue
			}
			seen[root] = struct{}{}
			candidates = append(candidates, sourceCandidate{kind: SourceProject, path: filepath.Join(root, ".awaken", "hooks.json"), projectRoot: root})
			candidates = append(candidates, sourceCandidate{kind: SourceProjectLocal, path: filepath.Join(root, ".awaken", "hooks.local.json"), projectRoot: root})
		}
	}
	seenStableIDs := map[string]struct{}{}
	for _, candidate := range candidates {
		receipt, config, err := s.loadCandidate(projectID, candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if includeReview && err == nil && config != nil {
			receipt.ReviewHooks = reviewHooks(*config)
		}
		snapshot.Sources = append(snapshot.Sources, receipt)
		if err != nil {
			snapshot.Diagnostics = append(snapshot.Diagnostics, Diagnostic{Code: "hook_config_invalid", Message: err.Error(), SourcePath: candidate.path})
			if candidate.kind == SourceUser {
				return snapshot, err
			}
			continue
		}
		if receipt.Status != StatusEnabled || config == nil {
			continue
		}
		events := make([]string, 0, len(config.Hooks))
		for event := range config.Hooks {
			events = append(events, string(event))
		}
		sort.Strings(events)
		for _, eventName := range events {
			event := Event(eventName)
			for _, group := range config.Hooks[event] {
				for _, hook := range group.Hooks {
					stableID := string(candidate.kind) + ":" + candidate.path + ":" + hook.ID
					if _, duplicate := seenStableIDs[stableID]; duplicate {
						return snapshot, fmt.Errorf("hookruntime: duplicate hook identity %q", stableID)
					}
					seenStableIDs[stableID] = struct{}{}
					snapshot.HooksByEvent[event] = append(snapshot.HooksByEvent[event], CompiledHook{StableID: stableID, Event: event, Matcher: group.Matcher, Command: hook, SourceKind: candidate.kind, SourcePath: candidate.path, ProjectRoot: candidate.projectRoot})
				}
			}
		}
	}
	snapshot.SnapshotID = digestSnapshot(snapshot)
	return snapshot, nil
}

func (s *Store) Inspect(projectID string, projectRoots []string, sessionWorkspace bool) (AvailabilitySnapshot, error) {
	return s.buildSnapshot(projectID, projectRoots, sessionWorkspace, true)
}

func (s *Store) loadCandidate(projectID string, candidate sourceCandidate) (SourceReceipt, *Config, error) {
	receipt := SourceReceipt{Kind: candidate.kind, Path: candidate.path, ProjectRoot: candidate.projectRoot}
	data, canonical, err := readConfigFile(candidate.path)
	receipt.Path = canonical
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return receipt, nil, err
		}
		receipt.Status, receipt.Diagnostic = StatusInvalid, err.Error()
		return receipt, nil, err
	}
	if candidate.kind.RequiresProjectTrust() {
		canonicalRoot, rootErr := filepath.EvalSymlinks(candidate.projectRoot)
		if rootErr != nil {
			err = fmt.Errorf("hookruntime: resolve project root: %w", rootErr)
			receipt.Status, receipt.Diagnostic = StatusInvalid, err.Error()
			return receipt, nil, err
		}
		canonicalRoot, rootErr = filepath.Abs(canonicalRoot)
		if rootErr != nil {
			receipt.Status, receipt.Diagnostic = StatusInvalid, rootErr.Error()
			return receipt, nil, rootErr
		}
		filename := "hooks.json"
		if candidate.kind == SourceProjectLocal {
			filename = "hooks.local.json"
		}
		expected := filepath.Join(filepath.Clean(canonicalRoot), ".awaken", filename)
		if canonical != expected {
			err = errors.New("hookruntime: project hooks.json resolves outside the project root")
			receipt.Status, receipt.Diagnostic = StatusInvalid, err.Error()
			return receipt, nil, err
		}
		receipt.ProjectRoot = filepath.Clean(canonicalRoot)
	}
	config, err := decodeConfig(data)
	if err != nil {
		receipt.Status, receipt.Diagnostic = StatusInvalid, err.Error()
		return receipt, nil, err
	}
	if candidate.kind == SourceUser {
		info, statErr := os.Stat(canonical)
		if statErr != nil {
			receipt.Status, receipt.Diagnostic = StatusInvalid, statErr.Error()
			return receipt, nil, statErr
		}
		if info.Mode().Perm()&0o077 != 0 {
			err = errors.New("hookruntime: user hooks.json permissions must not grant group or other access")
			receipt.Status, receipt.Diagnostic = StatusInvalid, err.Error()
			return receipt, nil, err
		}
	}
	normalized, _ := json.Marshal(config)
	receipt.Digest = digestBytes(normalized)
	receipt.HookCount = countHooks(config)
	if !config.Enabled {
		receipt.Status = StatusDisabled
		return receipt, &config, nil
	}
	if candidate.kind.RequiresProjectTrust() {
		trusted, trustErr := s.trust.IsTrusted(projectID, canonical, receipt.Digest)
		if trustErr != nil {
			receipt.Status, receipt.Diagnostic = StatusUntrusted, trustErr.Error()
			return receipt, &config, nil
		}
		if !trusted {
			receipt.Status = StatusUntrusted
			return receipt, &config, nil
		}
	}
	receipt.Status = StatusEnabled
	return receipt, &config, nil
}

func readConfigFile(path string) ([]byte, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, path, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, path, errors.New("hookruntime: config must be a regular non-symlink file")
	}
	if info.Size() > MaxConfigBytes {
		return nil, path, errors.New("hookruntime: config exceeds 1 MiB")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, path, fmt.Errorf("hookruntime: resolve config: %w", err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, path, err
	}
	file, err := os.Open(canonical)
	if err != nil {
		return nil, canonical, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, canonical, err
	}
	if !os.SameFile(info, openedInfo) || !openedInfo.Mode().IsRegular() {
		return nil, canonical, errors.New("hookruntime: config identity changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxConfigBytes+1))
	if err != nil {
		return nil, canonical, err
	}
	if len(data) > MaxConfigBytes {
		return nil, canonical, errors.New("hookruntime: config exceeds 1 MiB")
	}
	if !utf8.Valid(data) {
		return nil, canonical, errors.New("hookruntime: config is not UTF-8")
	}
	return data, filepath.Clean(canonical), nil
}

func decodeConfig(data []byte) (Config, error) {
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("hookruntime: invalid config JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Config{}, errors.New("hookruntime: config contains trailing JSON")
	}
	if config.Version != ConfigVersion {
		return Config{}, fmt.Errorf("hookruntime: unsupported config version %d", config.Version)
	}
	seen := map[string]struct{}{}
	for event, groups := range config.Hooks {
		if !validEvent(event) {
			return Config{}, fmt.Errorf("hookruntime: unsupported event %q", event)
		}
		for groupIndex := range groups {
			group := &groups[groupIndex]
			if err := normalizeMatcher(event, &group.Matcher); err != nil {
				return Config{}, err
			}
			if len(group.Hooks) == 0 {
				return Config{}, fmt.Errorf("hookruntime: event %s contains an empty hooks list", event)
			}
			for hookIndex := range group.Hooks {
				hook := &group.Hooks[hookIndex]
				if err := normalizeHook(event, hook); err != nil {
					return Config{}, err
				}
				if _, ok := seen[hook.ID]; ok {
					return Config{}, fmt.Errorf("hookruntime: duplicate hook id %q", hook.ID)
				}
				seen[hook.ID] = struct{}{}
			}
			groups[groupIndex] = *group
		}
		config.Hooks[event] = groups
	}
	return config, nil
}

// CompileActivatedSkillHooks validates the same event/executor schema used by
// hooks.json, then anchors command argv[0] beneath the activated package root.
// It performs no execution and must only be called after successful activation.
func CompileActivatedSkillHooks(skillID, packageRoot, sourcePath string, source SourceKind, raw any) ([]CompiledHook, error) {
	encoded, err := json.Marshal(map[string]any{"version": ConfigVersion, "enabled": true, "hooks": raw})
	if err != nil {
		return nil, err
	}
	config, err := decodeConfig(encoded)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(packageRoot)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	var compiled []CompiledHook
	for event, groups := range config.Hooks {
		for _, group := range groups {
			for _, command := range group.Hooks {
				if command.Type == "command" {
					if filepath.IsAbs(command.Command[0]) {
						return nil, errors.New("hookruntime: Skill command must be package-relative")
					}
					candidate, resolveErr := filepath.EvalSymlinks(filepath.Join(root, command.Command[0]))
					if resolveErr != nil || !pathWithin(root, candidate) {
						return nil, errors.New("hookruntime: Skill command resolves outside package root")
					}
					command.Command[0] = candidate
					command.CWD = "project_root"
				}
				compiled = append(compiled, CompiledHook{StableID: "skill:" + skillID + ":" + command.ID, Event: event, Matcher: group.Matcher, Command: command, SourceKind: source, SourcePath: sourcePath, ProjectRoot: root})
			}
		}
	}
	return compiled, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func validEvent(event Event) bool {
	switch event {
	case EventRunStart, EventUserPromptSubmit, EventPreToolUse, EventPostToolUse, EventPostToolUseFailure,
		EventPreCompact, EventPostCompact, EventSubagentStart, EventSubagentStop, EventBeforeFinish, EventRunEnd:
		return true
	}
	return false
}

func normalizeMatcher(event Event, matcher *Matcher) error {
	toolEvent := event == EventPreToolUse || event == EventPostToolUse || event == EventPostToolUseFailure
	if !toolEvent && len(matcher.ToolNames) > 0 {
		return fmt.Errorf("hookruntime: event %s does not accept toolNames matcher", event)
	}
	if toolEvent && len(matcher.ToolNames) == 0 {
		return fmt.Errorf("hookruntime: event %s requires toolNames matcher", event)
	}
	seen := map[string]struct{}{}
	for index, name := range matcher.ToolNames {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "|^$[]()\\") {
			return fmt.Errorf("hookruntime: invalid exact tool matcher %q", name)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("hookruntime: duplicate tool matcher %q", name)
		}
		seen[name] = struct{}{}
		matcher.ToolNames[index] = name
	}
	return nil
}

func normalizeHook(event Event, hook *CommandHook) error {
	if !hookIDPattern.MatchString(hook.ID) {
		return fmt.Errorf("hookruntime: invalid hook id %q", hook.ID)
	}
	if hook.Type != "command" && hook.Type != "http" && hook.Type != "prompt" && hook.Type != "agent" {
		return fmt.Errorf("hookruntime: unsupported hook type %q", hook.Type)
	}
	if hook.Type == "command" && (len(hook.Command) == 0 || strings.TrimSpace(hook.Command[0]) == "") {
		return fmt.Errorf("hookruntime: hook %s command must be a non-empty argv array", hook.ID)
	}
	for _, arg := range hook.Command {
		if strings.ContainsRune(arg, 0) {
			return fmt.Errorf("hookruntime: hook %s command contains NUL", hook.ID)
		}
	}
	if hook.Type != "command" && (len(hook.Command) > 0 || hook.CWD != "") {
		return fmt.Errorf("hookruntime: hook %s command/cwd fields require type command", hook.ID)
	}
	if hook.Type == "command" && hook.CWD == "" {
		hook.CWD = "workspace_root"
	}
	if hook.Type == "command" && hook.CWD != "workspace_root" && hook.CWD != "project_root" {
		return fmt.Errorf("hookruntime: hook %s has unsupported cwd %q", hook.ID, hook.CWD)
	}
	if err := normalizeExecutorFields(hook); err != nil {
		return err
	}
	if hook.TimeoutMS == 0 {
		hook.TimeoutMS = 10000
	}
	if hook.TimeoutMS < 100 || hook.TimeoutMS > 120000 {
		return fmt.Errorf("hookruntime: hook %s timeoutMs out of range", hook.ID)
	}
	if hook.MaxOutputBytes == 0 {
		hook.MaxOutputBytes = 65536
	}
	if hook.MaxOutputBytes < 1 || hook.MaxOutputBytes > MaxConfigBytes {
		return fmt.Errorf("hookruntime: hook %s maxOutputBytes out of range", hook.ID)
	}
	if hook.OnFailure == "" {
		hook.OnFailure = "continue"
	}
	if hook.OnFailure != "continue" && hook.OnFailure != "block" {
		return fmt.Errorf("hookruntime: hook %s has invalid onFailure", hook.ID)
	}
	if hook.OnFailure == "block" && !canBlock(event) {
		return fmt.Errorf("hookruntime: event %s cannot block on failure", event)
	}
	return nil
}

func normalizeExecutorFields(hook *CommandHook) error {
	switch hook.Type {
	case "command":
		if hook.URL != "" || hook.Prompt != "" || hook.Model != "" || hook.MaxTurns != 0 || len(hook.SecretHeaders) > 0 || hook.AllowLoopback {
			return fmt.Errorf("hookruntime: hook %s contains fields for another executor", hook.ID)
		}
	case "http":
		parsed, err := url.Parse(strings.TrimSpace(hook.URL))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
			return fmt.Errorf("hookruntime: hook %s has invalid http URL", hook.ID)
		}
		if hook.Prompt != "" || hook.Model != "" || hook.MaxTurns != 0 {
			return fmt.Errorf("hookruntime: hook %s contains fields for another executor", hook.ID)
		}
		for name, ref := range hook.SecretHeaders {
			if !validHeaderName(name) || !hookIDPattern.MatchString(strings.TrimSpace(ref)) {
				return fmt.Errorf("hookruntime: hook %s has invalid secret header reference", hook.ID)
			}
		}
	case "prompt", "agent":
		if strings.TrimSpace(hook.Prompt) == "" || len([]byte(hook.Prompt)) > 32*1024 {
			return fmt.Errorf("hookruntime: hook %s prompt must be 1..32768 bytes", hook.ID)
		}
		if hook.URL != "" || len(hook.SecretHeaders) > 0 || hook.AllowLoopback {
			return fmt.Errorf("hookruntime: hook %s contains fields for another executor", hook.ID)
		}
		if hook.Type == "prompt" && hook.MaxTurns != 0 {
			return fmt.Errorf("hookruntime: hook %s maxTurns requires type agent", hook.ID)
		}
		if hook.Type == "agent" {
			if hook.MaxTurns == 0 {
				hook.MaxTurns = 8
			}
			if hook.MaxTurns < 1 || hook.MaxTurns > 8 {
				return fmt.Errorf("hookruntime: hook %s maxTurns out of range", hook.ID)
			}
		}
	}
	return nil
}

func validHeaderName(value string) bool {
	if value == "" || strings.EqualFold(value, "Host") || strings.EqualFold(value, "Content-Length") {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
			return false
		}
	}
	return true
}

func countHooks(config Config) int {
	total := 0
	for _, groups := range config.Hooks {
		for _, group := range groups {
			total += len(group.Hooks)
		}
	}
	return total
}

func reviewHooks(config Config) []ReviewHook {
	events := make([]string, 0, len(config.Hooks))
	for event := range config.Hooks {
		events = append(events, string(event))
	}
	sort.Strings(events)
	review := make([]ReviewHook, 0, countHooks(config))
	for _, eventName := range events {
		event := Event(eventName)
		for _, group := range config.Hooks[event] {
			for _, hook := range group.Hooks {
				headerNames := make([]string, 0, len(hook.SecretHeaders))
				for name := range hook.SecretHeaders {
					headerNames = append(headerNames, name)
				}
				sort.Strings(headerNames)
				review = append(review, ReviewHook{
					ID:                hook.ID,
					Event:             event,
					Type:              hook.Type,
					ToolNames:         append([]string(nil), group.Matcher.ToolNames...),
					Command:           append([]string(nil), hook.Command...),
					CWD:               hook.CWD,
					URL:               hook.URL,
					AllowLoopback:     hook.AllowLoopback,
					SecretHeaderNames: headerNames,
					Prompt:            hook.Prompt,
					Model:             hook.Model,
					MaxTurns:          hook.MaxTurns,
					TimeoutMS:         hook.TimeoutMS,
					MaxOutputBytes:    hook.MaxOutputBytes,
					OnFailure:         hook.OnFailure,
				})
			}
		}
	}
	return review
}
func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func digestSnapshot(snapshot AvailabilitySnapshot) string {
	snapshot.CreatedAt = snapshot.CreatedAt.Truncate(0)
	data, _ := json.Marshal(struct {
		Version int                      `json:"version"`
		Sources []SourceReceipt          `json:"sources"`
		Hooks   map[Event][]CompiledHook `json:"hooks"`
	}{snapshot.SchemaVersion, snapshot.Sources, snapshot.HooksByEvent})
	return digestBytes(data)
}
