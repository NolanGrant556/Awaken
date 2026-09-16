// Package managedenv owns reusable, user-scoped execution dependencies.
// It does not depend on the model, Electron, or a business workspace.
package managedenv

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = 1

type Requirement struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
}
type Request struct {
	Capability     string        `json:"capability,omitempty"`
	Requirements   []Requirement `json:"requirements,omitempty"`
	EnvironmentRef string        `json:"environment_ref,omitempty"`
	Purpose        string        `json:"purpose,omitempty"`
}
type Spec struct {
	OfficeBundle      *OfficeBundleIdentity `json:"office_bundle,omitempty"`
	Components        []string              `json:"components,omitempty"`
	ComponentVersions map[string]string     `json:"component_versions,omitempty"`
	Python            map[string]string     `json:"python,omitempty"`
	Node              map[string]string     `json:"node,omitempty"`
}
type Asset struct {
	URL              string            `json:"url"`
	SHA256           string            `json:"sha256"`
	Format           string            `json:"format"`
	StripPrefix      string            `json:"strip_prefix,omitempty"`
	Executables      map[string]string `json:"executables"`
	Resources        map[string]string `json:"resources,omitempty"`
	MaxBytes         int64             `json:"max_bytes,omitempty"`
	MaxExpandedBytes int64             `json:"max_expanded_bytes,omitempty"`
}
type Component struct {
	Version   string           `json:"version"`
	License   string           `json:"license"`
	Source    string           `json:"source"`
	Assets    map[string]Asset `json:"assets"`
	Probe     string           `json:"probe,omitempty"`
	ProbeArgs []string         `json:"probe_args,omitempty"`
	Optional  bool             `json:"optional,omitempty"`
}
type Catalog struct {
	SchemaVersion int                  `json:"schema_version"`
	Revision      string               `json:"revision"`
	Components    map[string]Component `json:"components"`
	Capabilities  map[string]Spec      `json:"capabilities"`
	OfficeBundle  *OfficeBundleRelease `json:"office_bundle,omitempty"`
}
type Owner struct {
	ThreadID string `json:"thread_id"`
	AgentID  string `json:"agent_id"`
	RunID    string `json:"run_id,omitempty"`
	TaskID   string `json:"task_id,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

func (o Owner) key() string { return digest([]string{o.ThreadID, o.AgentID}) }
func (o Owner) valid() bool { return o.ThreadID != "" && o.AgentID != "" }

type Launch struct {
	Executable        string   `json:"executable"`
	Args              []string `json:"args,omitempty"`
	ManagedInstanceID string   `json:"-"`
}
type Binding struct {
	State          string            `json:"state"`
	EnvironmentRef string            `json:"environment_ref"`
	InstanceID     string            `json:"instance_id"`
	Reused         bool              `json:"reused"`
	Spec           Spec              `json:"spec"`
	Executables    map[string]Launch `json:"executables"`
	Resources      map[string]string `json:"resources,omitempty"`
	Environment    map[string]string `json:"environment"`
	Owner          Owner             `json:"-"`
	BundleVersion  string            `json:"bundle_version,omitempty"`
}
type Manifest struct {
	OwnedFiles          map[string]string `json:"owned_files,omitempty"` // Imported CLI files outside the instance root, with deletion hashes.
	RepairOf            string            `json:"repair_of,omitempty"`
	SchemaVersion       int               `json:"schema_version"`
	Key                 string            `json:"key"`
	InstanceID          string            `json:"instance_id"`
	Kind                string            `json:"kind"`
	ComponentID         string            `json:"component_id,omitempty"`
	Version             string            `json:"version,omitempty"`
	Target              string            `json:"target"`
	Source              string            `json:"source"`
	Root                string            `json:"root"`
	State               string            `json:"state"`
	Spec                Spec              `json:"spec"`
	Components          []string          `json:"components,omitempty"`
	Executables         map[string]Launch `json:"executables"`
	Resources           map[string]string `json:"resources,omitempty"`
	Environment         map[string]string `json:"environment,omitempty"`
	Fingerprints        map[string]string `json:"fingerprints"`
	NodeLauncherVersion int               `json:"node_launcher_version,omitempty"`
	LockDigest          string            `json:"lock_digest,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	LastUsedAt          time.Time         `json:"last_used_at"`
	RetiredAt           *time.Time        `json:"retired_at,omitempty"`
}
type Operation struct {
	RepairOf        string `json:"repair_of,omitempty"`
	BundleVersion   string `json:"bundle_version,omitempty"`
	started         bool
	ID              string    `json:"operation_id"`
	RequestKey      string    `json:"request_key"`
	Attempt         string    `json:"attempt_id"`
	CoreID          string    `json:"core_id"`
	State           string    `json:"state"`
	Component       string    `json:"component,omitempty"`
	Message         string    `json:"message"`
	DownloadedBytes int64     `json:"downloaded_bytes"`
	TotalBytes      int64     `json:"total_bytes,omitempty"`
	Downloads       int       `json:"downloads"`
	Installs        int       `json:"installs"`
	ErrorCode       string    `json:"error_code,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type Progress func(Operation)
type Options struct {
	Root            string
	Catalog         Catalog
	Target          string
	Client          *http.Client `json:"-"`
	AllowFileAssets bool         // Explicit fixture mode, never enabled by the product.
	Timeout         time.Duration
	Now             func() time.Time `json:"-"`
}
type Error struct{ Code, Message string }

func (e *Error) Error() string           { return e.Code + ": " + e.Message }
func failure(code, message string) error { return &Error{code, message} }

func digest(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
var safePython = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*(?:\[[a-zA-Z0-9_,.-]+\])?$`)
var safeNode = regexp.MustCompile(`^(?:@[a-zA-Z0-9._-]+/)?[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var safeVersion = regexp.MustCompile(`^[0-9vV.*<>=!~^, |]{0,80}$`)
var numericVersion = regexp.MustCompile(`^[vV]?[0-9]+(?:\.[0-9]+){0,5}(?:\.\*)?$`)
var pythonSeparator = regexp.MustCompile(`[-_.]+`)

func normalizePythonName(name string) string {
	parts := strings.SplitN(strings.ToLower(name), "[", 2)
	base := pythonSeparator.ReplaceAllString(parts[0], "-")
	if len(parts) == 2 {
		extras := strings.Split(strings.TrimSuffix(parts[1], "]"), ",")
		sort.Strings(extras)
		return base + "[" + strings.Join(unique(extras), ",") + "]"
	}
	return base
}
func parseVersion(raw string) ([]int, bool) {
	if !numericVersion.MatchString(raw) {
		return nil, false
	}
	raw = strings.TrimLeft(raw, "vV")
	raw = strings.TrimSuffix(raw, ".*")
	var out []int
	for _, part := range strings.Split(raw, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}
func constraintParts(constraint string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}
	for _, group := range strings.Split(constraint, "||") {
		parts := strings.Fields(strings.ReplaceAll(group, ",", " "))
		if len(parts) == 0 {
			return false
		}
		for _, part := range parts {
			part = strings.TrimLeft(part, "<>=!~^")
			if _, ok := parseVersion(part); !ok {
				return false
			}
		}
	}
	return true
}

// Legacy Office capability IDs remain executable for old callers, but are no
// longer choices offered to the model alongside the fixed Office bundle.
func (c Catalog) AdvertisedCapabilities() []string {
	names := make([]string, 0, len(c.Capabilities))
	for name := range c.Capabilities {
		if name == "python-office" || name == "node-office" || name == "office-render" || name == "pdf_max" || name == "creator" || strings.HasPrefix(name, "word.") || strings.HasPrefix(name, "excel.") || strings.HasPrefix(name, "ppt.") || strings.HasPrefix(name, "pdf.") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c Catalog) Resolve(req Request) (Spec, error) {
	if (req.Capability != "" && len(req.Requirements) > 0) || (req.Capability == "" && len(req.Requirements) == 0) {
		return Spec{}, failure("environment_request_invalid", "provide capability or requirements")
	}
	s := Spec{Python: map[string]string{}, Node: map[string]string{}}
	if req.Capability != "" {
		v, ok := c.Capabilities[req.Capability]
		if !ok {
			return s, failure("environment_capability_unknown", req.Capability+"; Office/PDF/creator tasks use load_workspace_dependencies; extension capabilities: "+strings.Join(c.AdvertisedCapabilities(), ", "))
		}
		b, _ := json.Marshal(v)
		_ = json.Unmarshal(b, &s)
		s.Python = canonicalPython(s.Python)
	} else {
		if len(req.Requirements) > 64 {
			return s, failure("environment_request_invalid", "too many requirements")
		}
		seen := map[string]string{}
		for _, r := range req.Requirements {
			if r.Ecosystem == "python" {
				r.Name = normalizePythonName(r.Name)
			}
			key := r.Ecosystem + ":" + r.Name
			if previous, ok := seen[key]; ok && previous != r.Version {
				return s, failure("environment_request_invalid", "conflicting duplicate requirements")
			}
			seen[key] = r.Version
			if r.Version != strings.TrimSpace(r.Version) {
				return s, failure("environment_request_invalid", "invalid version constraint")
			}
			if !safeVersion.MatchString(r.Version) || !constraintParts(r.Version) {
				return s, failure("environment_request_invalid", "invalid version constraint")
			}
			switch r.Ecosystem {
			case "component":
				if !safeID.MatchString(r.Name) {
					return s, failure("environment_request_invalid", "invalid component")
				}
				if r.Version != "" {
					v, ok := c.Components[r.Name]
					if !ok || !satisfies(v.Version, r.Version) {
						return s, failure("environment_version_mismatch", r.Name)
					}
				}
				s.Components = append(s.Components, r.Name)
				if r.Version != "" {
					if s.ComponentVersions == nil {
						s.ComponentVersions = map[string]string{}
					}
					s.ComponentVersions[r.Name] = r.Version
				}
			case "python":
				if !safePython.MatchString(r.Name) {
					return s, failure("environment_request_invalid", "invalid Python package")
				}
				s.Python[strings.ToLower(r.Name)] = r.Version
			case "node":
				if !safeNode.MatchString(r.Name) {
					return s, failure("environment_request_invalid", "invalid Node package")
				}
				if parts, ok := parseVersion(r.Version); ok && len(parts) < 3 && !strings.Contains(r.Version, "*") {
					r.Version += ".*"
				}
				s.Node[r.Name] = r.Version
			default:
				return s, failure("environment_request_invalid", "unknown ecosystem")
			}
		}
	}
	if len(s.Python) > 0 {
		// uv is needed to install packages, not to run an already prepared
		// package set. Legacy Office capabilities also listed it implicitly.
		if req.Capability != "" {
			components := s.Components[:0]
			for _, id := range s.Components {
				if id != "uv" {
					components = append(components, id)
				}
			}
			s.Components = components
		}
		s.Components = append(s.Components, "python")
	}
	if len(s.Node) > 0 {
		s.Components = append(s.Components, "node")
	}
	sort.Strings(s.Components)
	s.Components = unique(s.Components)
	for _, id := range s.Components {
		if _, ok := c.Components[id]; !ok {
			return s, failure("environment_source_unavailable", id)
		}
	}
	return s, nil
}

func canonicalPython(packages map[string]string) map[string]string {
	if len(packages) == 0 {
		return nil
	}
	result := make(map[string]string, len(packages))
	for name, version := range packages {
		result[normalizePythonName(name)] = version
	}
	return result
}
func unique(v []string) []string {
	out := []string{}
	for _, s := range v {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}
func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func specContains(have, want Spec) bool {
	if want.OfficeBundle != nil && (have.OfficeBundle == nil || *have.OfficeBundle != *want.OfficeBundle) {
		return false
	}
	have.Python = canonicalPython(have.Python)
	want.Python = canonicalPython(want.Python)
	for _, id := range want.Components {
		if !contains(have.Components, id) {
			return false
		}
	}
	for n, v := range want.ComponentVersions {
		if actual, ok := have.ComponentVersions[n]; !ok || !satisfies(actual, v) {
			return false
		}
	}
	for n, v := range want.Python {
		if h, ok := have.Python[n]; !ok || !satisfies(h, v) {
			return false
		}
	}
	for n, v := range want.Node {
		if h, ok := have.Node[n]; !ok || !satisfies(h, v) {
			return false
		}
	}
	return true
}

// constraints use the common numeric subset; unsupported syntax is never
// interpreted as compatibility. Lock resolution is performed by uv/npm.
func satisfies(version, constraint string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}
	if version == constraint {
		return true
	}
	actualParts, valid := parseVersion(version)
	if !valid {
		return false
	}
	var actual [6]int
	copy(actual[:], actualParts)
	for _, or := range strings.Split(constraint, "||") {
		ok := true
		for _, part := range strings.Fields(strings.ReplaceAll(or, ",", " ")) {
			op := "="
			raw := part
			for _, p := range []string{">=", "<=", "!=", "==", "~=", ">", "<", "^", "~", "="} {
				if strings.HasPrefix(part, p) {
					op = p
					raw = strings.TrimPrefix(part, p)
					break
				}
			}
			wantedParts, valid := parseVersion(raw)
			if !valid {
				ok = false
				break
			}
			var wanted [6]int
			copy(wanted[:], wantedParts)
			count := len(wantedParts)

			cmp := 0
			for i := 0; i < 6; i++ {
				if actual[i] < wanted[i] {
					cmp = -1
					break
				}
				if actual[i] > wanted[i] {
					cmp = 1
					break
				}
			}
			match := false
			switch op {
			case "=", "==", "!=":
				match = cmp == 0
				if strings.Contains(raw, "*") {
					match = true
					for i := 0; i < count; i++ {
						match = match && actual[i] == wanted[i]
					}
				}
				if op == "!=" {
					match = !match
				}
			case ">=":
				match = cmp >= 0
			case "<=":
				match = cmp <= 0
			case ">":
				match = cmp > 0
			case "<":
				match = cmp < 0
			case "^":
				match = cmp >= 0 && actual[0] == wanted[0] && (count < 2 || wanted[0] != 0 || actual[1] == wanted[1]) && (count < 3 || wanted[0] != 0 || wanted[1] != 0 || actual[2] == wanted[2])
			case "~":
				match = cmp >= 0 && actual[0] == wanted[0] && (count < 2 || actual[1] == wanted[1])
			case "~=":
				match = cmp >= 0 && actual[0] == wanted[0] && (count < 3 || actual[1] == wanted[1])
			}
			ok = ok && match
		}
		if ok {
			return true
		}
	}
	return false
}

type managerContextKey struct{}

func WithManager(ctx context.Context, m *Manager) context.Context {
	return context.WithValue(ctx, managerContextKey{}, m)
}
