package toolsafety

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const findingKindPath = "path"

// DefaultPolicy builds the default sensitive path deny policy.
func DefaultPolicy() Policy {
	home, _ := os.UserHomeDir()
	base, _ := os.Getwd()
	return Policy{
		HomeDir:          home,
		BaseDir:          base,
		DeniedExactPaths: DefaultDeniedExactPaths(home),
		DeniedPrefixes:   DefaultDeniedPrefixes(home),
	}.Normalize()
}

// DefaultDeniedExactPaths returns exact sensitive paths denied by default.
func DefaultDeniedExactPaths(home string) []string {
	return []string{
		filepath.Join(home, ".ssh", "authorized_keys"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "config"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".bash_profile"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".zprofile"),
		filepath.Join(home, ".profile"),
		filepath.Join(home, ".netrc"),
		filepath.Join(home, ".npmrc"),
		filepath.Join(home, ".pypirc"),
		filepath.Join(home, ".pgpass"),
		"/etc/passwd",
		"/etc/shadow",
		"/etc/sudoers",
	}
}

// DefaultDeniedPrefixes returns sensitive directory prefixes denied by default.
func DefaultDeniedPrefixes(home string) []string {
	return []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".aws"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".docker"),
		filepath.Join(home, ".azure"),
		filepath.Join(home, ".config", "gh"),
		"/etc/sudoers.d",
		"/etc/systemd",
	}
}

// Normalize fills defaults and cleans configured paths.
func (p Policy) Normalize() Policy {
	if strings.TrimSpace(p.HomeDir) == "" {
		p.HomeDir, _ = os.UserHomeDir()
	}
	p.HomeDir = cleanAbsolute(expandHome(p.HomeDir, p.HomeDir), "")

	if strings.TrimSpace(p.BaseDir) == "" {
		p.BaseDir, _ = os.Getwd()
	}
	p.BaseDir = cleanAbsolute(expandHome(p.BaseDir, p.HomeDir), "")

	if p.DeniedExactPaths == nil {
		p.DeniedExactPaths = DefaultDeniedExactPaths(p.HomeDir)
	}
	if p.DeniedPrefixes == nil {
		p.DeniedPrefixes = DefaultDeniedPrefixes(p.HomeDir)
	}

	if strings.TrimSpace(p.SafeRoot) != "" {
		p.SafeRoot = cleanAbsolute(expandHome(p.SafeRoot, p.HomeDir), p.BaseDir)
		p.SafeRoot = resolveExistingPath(p.SafeRoot)
	}

	p.DeniedExactPaths = cleanPathList(p.DeniedExactPaths, p)
	p.DeniedPrefixes = cleanPathList(p.DeniedPrefixes, p)
	return p
}

// ClassifyPath returns an allow/deny finding for a filesystem path.
func ClassifyPath(operation Operation, path string, policy Policy) Finding {
	policy = policy.Normalize()
	resolved := resolveInputPath(path, policy)
	finding := Finding{
		Kind:         findingKindPath,
		Operation:    operation,
		Input:        path,
		Decision:     DecisionAllowed,
		Severity:     SeverityInfo,
		Reason:       "path is allowed",
		ResolvedPath: resolved,
	}

	if !isSupportedOperation(operation) {
		finding.Decision = DecisionDenied
		finding.Severity = SeverityDanger
		finding.Reason = fmt.Sprintf("unsupported path operation %q", operation)
		return finding
	}

	if exact, ok := matchesExactDeniedPath(resolved, policy); ok {
		finding.Decision = DecisionDenied
		finding.Severity = SeverityDanger
		finding.Reason = fmt.Sprintf("path matches denied exact path %s", exact)
		return finding
	}

	if prefix, ok := matchesDeniedPrefix(resolved, policy); ok {
		finding.Decision = DecisionDenied
		finding.Severity = SeverityDanger
		finding.Reason = fmt.Sprintf("path is inside denied prefix %s", prefix)
		return finding
	}

	if isMutationOperation(operation) && strings.TrimSpace(policy.SafeRoot) != "" && !pathWithinRoot(resolved, policy.SafeRoot) {
		finding.Decision = DecisionDenied
		finding.Severity = SeverityDanger
		finding.Reason = fmt.Sprintf("write/delete path is outside safe root %s", policy.SafeRoot)
		return finding
	}

	return finding
}

func cleanPathList(paths []string, policy Policy) []string {
	cleaned := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		resolved := resolveInputPath(path, policy)
		if !seen[resolved] {
			cleaned = append(cleaned, resolved)
			seen[resolved] = true
		}
	}
	return cleaned
}

func resolveInputPath(path string, policy Policy) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	path = expandHome(path, policy.HomeDir)
	cleaned := cleanAbsolute(path, policy.BaseDir)
	return resolveExistingPath(cleaned)
}

func expandHome(path string, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func cleanAbsolute(path string, base string) string {
	if !filepath.IsAbs(path) {
		if strings.TrimSpace(base) != "" {
			path = filepath.Join(base, path)
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func resolveExistingPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved)
	}

	volume := filepath.VolumeName(path)
	current := filepath.Clean(path)
	var rest []string
	for {
		parent := filepath.Dir(current)
		if current == parent || current == volume+string(os.PathSeparator) || current == volume {
			break
		}
		rest = append([]string{filepath.Base(current)}, rest...)
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			parts := append([]string{resolved}, rest...)
			return filepath.Clean(filepath.Join(parts...))
		}
		current = parent
	}
	return filepath.Clean(path)
}

func matchesExactDeniedPath(path string, policy Policy) (string, bool) {
	for _, denied := range policy.DeniedExactPaths {
		if samePath(path, denied) {
			return denied, true
		}
	}
	return "", false
}

func matchesDeniedPrefix(path string, policy Policy) (string, bool) {
	for _, prefix := range policy.DeniedPrefixes {
		if pathWithinRoot(path, prefix) {
			return prefix, true
		}
	}
	return "", false
}

func samePath(a string, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func pathWithinRoot(path string, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func isSupportedOperation(operation Operation) bool {
	switch operation {
	case OperationRead, OperationWrite, OperationDelete:
		return true
	default:
		return false
	}
}

func isMutationOperation(operation Operation) bool {
	return operation == OperationWrite || operation == OperationDelete
}
