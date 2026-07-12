package skillruntime

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/skills"
	"awaken/src/tool"
)

// Compatibility status values for SkillActivationPlan.
const (
	CompatibilityOK     = "ok"
	CompatibilityFailed = "failed"
)

// SkillCompatibilityError is a structured activation failure.
type SkillCompatibilityError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e SkillCompatibilityError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// SkillResourceReceipt records a host-loaded skill package resource.
// Durable JSON uses snake_case; AG-UI boundaries may map to camelCase.
type SkillResourceReceipt struct {
	SkillID      string    `json:"skill_id"`
	SkillVersion string    `json:"skill_version"`
	Resource     string    `json:"resource"`
	Source       string    `json:"source"`
	SHA256       string    `json:"sha256"`
	Size         int64     `json:"size"`
	Mode         string    `json:"mode"`
	LoadedAt     time.Time `json:"loaded_at"`
	Content      string    `json:"-"`
}

// SkillActivationPlan is the compiled activation product for one skill.
type SkillActivationPlan struct {
	SkillID                   string                    `json:"skill_id"`
	SkillVersion              string                    `json:"skill_version"`
	EffectiveTools            []string                  `json:"effective_tools"`
	RequiredTools             []string                  `json:"required_tools,omitempty"`
	RequiredCapabilities      []tool.CapabilityClass    `json:"required_capabilities,omitempty"`
	RequiredRuntimeComponents []string                  `json:"required_runtime_components,omitempty"`
	ResourceReceipts          []SkillResourceReceipt    `json:"resource_receipts,omitempty"`
	CompatibilityStatus       string                    `json:"compatibility_status"`
	CompatibilityErrors       []SkillCompatibilityError `json:"compatibility_errors,omitempty"`
	MainBody                  string                    `json:"-"`
	Source                    skills.SourceOrigin       `json:"source,omitempty"`
	RootDir                   string                    `json:"-"`
	SkillFilePath             string                    `json:"skill_file_path,omitempty"`
	Context                   skills.ContextMode        `json:"context,omitempty"`
	Agent                     string                    `json:"agent,omitempty"`
	Model                     string                    `json:"model,omitempty"`
	Effort                    string                    `json:"effort,omitempty"`
	ToolHints                 []skills.ToolHint         `json:"tool_hints,omitempty"`
}

type InvocationSource string

const (
	InvocationUser  InvocationSource = "user"
	InvocationModel InvocationSource = "model"
)

// ActivationRequest configures a single skill activation.
type ActivationRequest struct {
	Context          context.Context
	SkillID          string
	ParentToolPool   []string
	Args             map[string]string
	SkipArgRender    bool
	RawArgs          string
	InvocationSource InvocationSource
}

// Activator compiles SkillActivationPlan values from the skill registry.
type Activator struct {
	Registry *skills.SkillRegistry
	Logger   Logger
}

// NewActivator returns an Activator bound to the given registry.
func NewActivator(registry *skills.SkillRegistry, logger Logger) *Activator {
	if logger == nil {
		logger = noopLogger{}
	}
	return &Activator{
		Registry: registry,
		Logger:   logger,
	}
}

// Activate compiles a SkillActivationPlan and loads SKILL.md on demand.
// Runtime, provider, tool and resource hints never disable a structurally
// valid Universal Skill at the catalog boundary.
func (a *Activator) Activate(req ActivationRequest) (*SkillActivationPlan, error) {
	if a == nil || a.Registry == nil {
		return nil, fmt.Errorf("skill activator unavailable")
	}
	skillID := strings.TrimSpace(req.SkillID)
	if skillID == "" {
		return nil, fmt.Errorf("skill_id is required")
	}
	def := a.Registry.Get(skillID)
	if def == nil {
		return nil, fmt.Errorf("skill %q not found", skillID)
	}
	if def.State == skills.StateInvalid || def.State == skills.StateShadowed || def.State == skills.StateDisabledByPolicy {
		code := strings.TrimSpace(def.UnavailableCode)
		if code == "" {
			code = CompatibilityFailed
		}
		return &SkillActivationPlan{
			SkillID:             skillID,
			SkillVersion:        strings.TrimSpace(def.Version),
			CompatibilityStatus: CompatibilityFailed,
			CompatibilityErrors: []SkillCompatibilityError{{
				Code:    code,
				Message: firstNonEmpty(def.UnavailableReason, "skill package is not loaded"),
			}},
			Source:  def.Source,
			RootDir: def.RootDir,
		}, &SkillCompatibilityError{Code: code, Message: firstNonEmpty(def.UnavailableReason, "skill package is not loaded")}
	}

	plan := &SkillActivationPlan{
		SkillID:                   skillID,
		SkillVersion:              strings.TrimSpace(def.Version),
		RequiredTools:             append([]string(nil), def.RequiredTools...),
		RequiredRuntimeComponents: append([]string(nil), def.RequiredRuntimeComponents...),
		Source:                    def.Source,
		RootDir:                   def.RootDir,
		SkillFilePath:             def.SkillFilePath,
		Context:                   def.Context,
		Agent:                     def.Agent,
		Model:                     def.Model,
		Effort:                    def.Effort,
		ToolHints:                 append([]skills.ToolHint(nil), def.ToolHints...),
		CompatibilityStatus:       CompatibilityOK,
	}
	for _, capName := range def.RequiredCapabilities {
		plan.RequiredCapabilities = append(plan.RequiredCapabilities, tool.CapabilityClass(strings.TrimSpace(capName)))
	}

	if req.InvocationSource == InvocationModel && def.DisableModelInvocation {
		err := SkillCompatibilityError{
			Code:    "skill_model_invocation_disabled",
			Message: fmt.Sprintf("skill %q can only be invoked explicitly by the user", skillID),
		}
		plan.CompatibilityStatus = CompatibilityFailed
		plan.CompatibilityErrors = append(plan.CompatibilityErrors, err)
		return plan, &err
	}

	var parentPool []string
	if req.ParentToolPool == nil {
		parentPool = CurrentParentToolPool()
	} else {
		// A non-nil empty slice is an explicit empty parent boundary. Do not
		// silently turn it into the process-global tool registry.
		parentPool = normalizeToolNames(req.ParentToolPool)
	}
	// A Skill may guide tool choice but never narrows the run. Keep the legacy
	// EffectiveTools field equal to the parent snapshot while upper layers
	// migrate away from that field.
	plan.EffectiveTools = normalizeToolNames(parentPool)

	body := def.Body
	if def.BodyFetcher != nil && def.McpBinding != nil {
		fetchCtx := req.Context
		if fetchCtx == nil {
			fetchCtx = context.Background()
		}
		fetched, fetchErr := def.BodyFetcher.FetchPromptBody(fetchCtx, def.McpBinding.ServerName, def.McpBinding.PromptName)
		if fetchErr != nil {
			plan.CompatibilityStatus = CompatibilityFailed
			cerr := SkillCompatibilityError{Code: CompatibilityFailed, Message: fetchErr.Error()}
			plan.CompatibilityErrors = append(plan.CompatibilityErrors, cerr)
			return plan, &cerr
		}
		body = fetched
	} else if strings.TrimSpace(def.SkillFilePath) != "" {
		loadedBody, loadErr := loadLocalSkillBody(def)
		if loadErr != nil {
			plan.CompatibilityStatus = CompatibilityFailed
			cerr := SkillCompatibilityError{Code: "skill_content_load_failed", Message: loadErr.Error()}
			plan.CompatibilityErrors = append(plan.CompatibilityErrors, cerr)
			return plan, &cerr
		}
		body = loadedBody
	}
	body = strings.ReplaceAll(body, "${AWAKEN_SKILL_DIR}", def.RootDir)
	if !req.SkipArgRender {
		argDefs := make([]SkillArgDef, len(def.Arguments))
		for i, arg := range def.Arguments {
			argDefs[i] = SkillArgDef{
				Name:        arg.Name,
				Type:        arg.Type,
				Description: arg.Description,
				Required:    arg.Required,
				Default:     arg.Default,
			}
		}
		rendered, renderErr := SubstituteArgs(body, skillID, argDefs, req.Args)
		if renderErr != nil {
			plan.CompatibilityStatus = CompatibilityFailed
			cerr := SkillCompatibilityError{Code: "missing_argument", Message: renderErr.Error()}
			plan.CompatibilityErrors = append(plan.CompatibilityErrors, cerr)
			return plan, &cerr
		}
		body = rendered
		body = substituteRawArgs(body, req.RawArgs)
	}
	plan.MainBody = renderActivatedBody(def, body)

	if len(plan.CompatibilityErrors) > 0 {
		plan.CompatibilityStatus = CompatibilityFailed
		first := plan.CompatibilityErrors[0]
		return plan, &first
	}
	return plan, nil
}

// Clone returns a deep copy suitable for handing across a run boundary. The
// activation plan is a per-call value; callers must never publish the mutable
// slices owned by an Activator invocation through process-global state.
func (p *SkillActivationPlan) Clone() *SkillActivationPlan {
	if p == nil {
		return nil
	}
	clone := *p
	clone.EffectiveTools = append([]string(nil), p.EffectiveTools...)
	clone.RequiredTools = append([]string(nil), p.RequiredTools...)
	clone.RequiredCapabilities = append([]tool.CapabilityClass(nil), p.RequiredCapabilities...)
	clone.RequiredRuntimeComponents = append([]string(nil), p.RequiredRuntimeComponents...)
	clone.ResourceReceipts = append([]SkillResourceReceipt(nil), p.ResourceReceipts...)
	clone.CompatibilityErrors = append([]SkillCompatibilityError(nil), p.CompatibilityErrors...)
	clone.ToolHints = append([]skills.ToolHint(nil), p.ToolHints...)
	return &clone
}

// DurablePayload returns the canonical snake_case activation payload shared by
// explicit and implicit activation journal writers.
func (p *SkillActivationPlan) DurablePayload() map[string]any {
	if p == nil {
		return nil
	}
	capabilities := make([]string, 0, len(p.RequiredCapabilities))
	for _, capability := range p.RequiredCapabilities {
		capabilities = append(capabilities, string(capability))
	}
	return map[string]any{
		"skill_id":                    p.SkillID,
		"skill_version":               p.SkillVersion,
		"source":                      string(p.Source),
		"effective_tools":             append([]string(nil), p.EffectiveTools...),
		"compatibility_status":        p.CompatibilityStatus,
		"compatibility_errors":        append([]SkillCompatibilityError(nil), p.CompatibilityErrors...),
		"required_tools":              append([]string(nil), p.RequiredTools...),
		"required_capabilities":       capabilities,
		"required_runtime_components": append([]string(nil), p.RequiredRuntimeComponents...),
		"skill_file_path":             p.SkillFilePath,
		"root_dir":                    p.RootDir,
		"context":                     string(p.Context),
		"agent":                       p.Agent,
		"model":                       p.Model,
		"effort":                      p.Effort,
		"tool_hints":                  append([]skills.ToolHint(nil), p.ToolHints...),
	}
}

// DurablePayload returns the canonical snake_case resource receipt payload.
func (r SkillResourceReceipt) DurablePayload() map[string]any {
	return map[string]any{
		"skill_id":      r.SkillID,
		"skill_version": r.SkillVersion,
		"resource":      r.Resource,
		"source":        r.Source,
		"sha256":        r.SHA256,
		"size":          r.Size,
		"mode":          r.Mode,
		"loaded_at":     r.LoadedAt.UTC().Format(time.RFC3339Nano),
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func renderActivatedBody(def *skills.SkillDefinition, body string) string {
	var b strings.Builder
	if root := strings.TrimSpace(def.RootDir); root != "" {
		b.WriteString("Base directory for this skill: ")
		b.WriteString(root)
		b.WriteString("\n")
		b.WriteString("Resources are not preloaded. Read references, scripts, templates, and assets from this package only when the task needs them, using the appropriate file, artifact, or shell tool.\n\n")
	}
	b.WriteString(body)
	return b.String()
}

func loadLocalSkillBody(def *skills.SkillDefinition) (string, error) {
	if def == nil || strings.TrimSpace(def.RootDir) == "" {
		return "", fmt.Errorf("skill package root is unavailable")
	}
	currentRoot, err := filepath.EvalSymlinks(def.RootDir)
	if err != nil {
		return "", fmt.Errorf("resolve current skill package root: %w", err)
	}
	trustedRoot := strings.TrimSpace(def.ResolvedRootDir)
	if trustedRoot != "" && filepath.Clean(currentRoot) != filepath.Clean(trustedRoot) {
		return "", fmt.Errorf("skill package root changed after catalog discovery")
	}
	root, err := os.OpenRoot(currentRoot)
	if err != nil {
		return "", fmt.Errorf("open skill package root: %w", err)
	}
	defer root.Close()
	entrypoint := "SKILL.md"
	if skillPath := strings.TrimSpace(def.SkillFilePath); skillPath != "" {
		resolvedSkillPath, resolveErr := filepath.EvalSymlinks(skillPath)
		if resolveErr != nil {
			return "", fmt.Errorf("resolve skill entrypoint: %w", resolveErr)
		}
		relative, relErr := filepath.Rel(currentRoot, resolvedSkillPath)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return "", fmt.Errorf("skill entrypoint escapes package root")
		}
		entrypoint = relative
	}
	file, err := root.Open(entrypoint)
	if err != nil {
		return "", fmt.Errorf("open SKILL.md: %w", err)
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("read SKILL.md: %w", err)
	}
	parsed, err := skills.ParseFrontmatter(string(raw))
	if err != nil {
		if def.Source == skills.SourceLegacy {
			return strings.TrimSpace(string(raw)), nil
		}
		return "", err
	}
	return parsed.Body, nil
}

func substituteRawArgs(body, rawArgs string) string {
	body = strings.ReplaceAll(body, "$ARGUMENTS", rawArgs)
	body = strings.ReplaceAll(body, "${ARGUMENTS}", rawArgs)
	return body
}

// CurrentParentToolPool returns the currently registered tool names as the
// default parent pool when a caller does not supply one.
func CurrentParentToolPool() []string {
	tools := tool.ListTools()
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
	}
	sort.Strings(names)
	return names
}

func normalizeToolNames(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
