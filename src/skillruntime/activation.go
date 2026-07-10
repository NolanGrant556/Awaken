package skillruntime

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"awaken/src/skills"
	"awaken/src/tool"
)

// Compatibility status values for SkillActivationPlan.
const (
	CompatibilityOK      = "ok"
	CompatibilityConflict = "conflict"
	CompatibilityFailed  = "failed"
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
	SkillID                   string                     `json:"skill_id"`
	SkillVersion              string                     `json:"skill_version"`
	EffectiveTools            []string                   `json:"effective_tools"`
	RequiredTools             []string                   `json:"required_tools,omitempty"`
	RequiredCapabilities      []tool.CapabilityClass     `json:"required_capabilities,omitempty"`
	RequiredRuntimeComponents []string                   `json:"required_runtime_components,omitempty"`
	ResourceReceipts          []SkillResourceReceipt     `json:"resource_receipts,omitempty"`
	CompatibilityStatus       string                     `json:"compatibility_status"`
	CompatibilityErrors       []SkillCompatibilityError  `json:"compatibility_errors,omitempty"`
	MainBody                  string                     `json:"-"`
	Source                    skills.SourceOrigin        `json:"source,omitempty"`
	RootDir                   string                     `json:"-"`
}

// ActivationRequest configures a single skill activation.
type ActivationRequest struct {
	SkillID          string
	ParentToolPool   []string
	Args             map[string]string
	SkipArgRender    bool
	RuntimeAvailable func(component string) bool
	ToolCapability   func(toolName string) (tool.CapabilityClass, bool)
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
	return &Activator{Registry: registry, Logger: logger}
}

// Activate compiles and validates a SkillActivationPlan. Required failures
// fail closed before the caller may issue the first LLM request.
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
	if def.State == skills.StateUnavailable || def.State == skills.StateShadowed {
		code := strings.TrimSpace(def.UnavailableCode)
		if code == "" {
			code = CompatibilityFailed
		}
		return &SkillActivationPlan{
			SkillID:             skillID,
			SkillVersion:        strings.TrimSpace(def.Version),
			CompatibilityStatus: CompatibilityConflict,
			CompatibilityErrors: []SkillCompatibilityError{{
				Code:    code,
				Message: firstNonEmpty(def.UnavailableReason, "skill is not available"),
			}},
			Source:  def.Source,
			RootDir: def.RootDir,
		}, &SkillCompatibilityError{Code: code, Message: firstNonEmpty(def.UnavailableReason, "skill is not available")}
	}

	plan := &SkillActivationPlan{
		SkillID:                   skillID,
		SkillVersion:              strings.TrimSpace(def.Version),
		RequiredTools:             append([]string(nil), def.RequiredTools...),
		RequiredRuntimeComponents: append([]string(nil), def.RequiredRuntimeComponents...),
		Source:                    def.Source,
		RootDir:                   def.RootDir,
		CompatibilityStatus:       CompatibilityOK,
	}
	for _, capName := range def.RequiredCapabilities {
		plan.RequiredCapabilities = append(plan.RequiredCapabilities, tool.CapabilityClass(strings.TrimSpace(capName)))
	}

	if conflicts := skills.DetectPlatformContractConflicts(def); len(conflicts) > 0 {
		err := SkillCompatibilityError{
			Code:    skills.ErrorCodePlatformContractConflict,
			Message: strings.Join(conflicts, "; "),
		}
		plan.CompatibilityStatus = CompatibilityConflict
		plan.CompatibilityErrors = append(plan.CompatibilityErrors, err)
		return plan, &err
	}

	parentPool := normalizeToolNames(req.ParentToolPool)
	if len(parentPool) == 0 {
		parentPool = CurrentParentToolPool()
	}
	effective, policyErrs := CompileEffectiveTools(parentPool, []skills.ToolPolicy{def.EffectiveToolPolicy()})
	plan.EffectiveTools = effective
	for _, pe := range policyErrs {
		plan.CompatibilityErrors = append(plan.CompatibilityErrors, pe)
	}

	capLookup := req.ToolCapability
	if capLookup == nil {
		capLookup = defaultToolCapability
	}
	plan.CompatibilityErrors = append(plan.CompatibilityErrors,
		ValidateRequiredTools(plan.EffectiveTools, def.RequiredTools)...)
	plan.CompatibilityErrors = append(plan.CompatibilityErrors,
		ValidateRequiredCapabilities(plan.EffectiveTools, plan.RequiredCapabilities, capLookup)...)

	runtimeOK := req.RuntimeAvailable
	if runtimeOK == nil {
		runtimeOK = func(string) bool { return true }
	}
	plan.CompatibilityErrors = append(plan.CompatibilityErrors,
		ValidateRequiredRuntimeComponents(def.RequiredRuntimeComponents, runtimeOK)...)

	receipts, loadErrs, loaded := LoadSkillResources(def, true)
	plan.ResourceReceipts = receipts
	plan.CompatibilityErrors = append(plan.CompatibilityErrors, loadErrs...)

	body := def.Body
	if def.BodyFetcher != nil && def.McpBinding != nil {
		fetched, fetchErr := def.BodyFetcher.FetchPromptBody(context.Background(), def.McpBinding.ServerName, def.McpBinding.PromptName)
		if fetchErr != nil {
			plan.CompatibilityStatus = CompatibilityFailed
			cerr := SkillCompatibilityError{Code: CompatibilityFailed, Message: fetchErr.Error()}
			plan.CompatibilityErrors = append(plan.CompatibilityErrors, cerr)
			return plan, &cerr
		}
		body = fetched
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
	}
	plan.MainBody = renderActivatedBody(def, body, loaded)

	if len(plan.CompatibilityErrors) > 0 {
		plan.CompatibilityStatus = CompatibilityFailed
		if plan.CompatibilityErrors[0].Code == skills.ErrorCodePlatformContractConflict {
			plan.CompatibilityStatus = CompatibilityConflict
		}
		first := plan.CompatibilityErrors[0]
		return plan, &first
	}
	return plan, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func renderActivatedBody(def *skills.SkillDefinition, body string, loaded map[string]string) string {
	var b strings.Builder
	if root := strings.TrimSpace(def.RootDir); root != "" {
		b.WriteString("Base directory for this skill: ")
		b.WriteString(root)
		b.WriteString("\n")
		b.WriteString("Required and preload resources are already loaded by the host. Use skill_resource_read only for optional on-demand package files.\n\n")
	}
	b.WriteString(body)
	if len(loaded) > 0 {
		keys := make([]string, 0, len(loaded))
		for k := range loaded {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("\n\n<awaken_skill_resources>\n")
		for _, k := range keys {
			b.WriteString("### ")
			b.WriteString(k)
			b.WriteString("\n")
			b.WriteString(loaded[k])
			b.WriteString("\n\n")
		}
		b.WriteString("</awaken_skill_resources>\n")
	}
	return b.String()
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

func defaultToolCapability(toolName string) (tool.CapabilityClass, bool) {
	t, err := tool.GetTool(toolName)
	if err != nil {
		return "", false
	}
	return t.Definition().Annotations.CapabilityClass, true
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
