package tool

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
)

var (
	toolRegistry     = make(map[string]RegisteredTool)
	registryVersion  uint64
	mu               sync.RWMutex
	ErrToolNotFound  = errors.New("tool not found")
	ErrDuplicateTool = errors.New("tool already registered")
	ErrInvalidTool   = errors.New("invalid tool")
)

var safeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// mcpSafeNamePattern extends the builtin safe-name pattern to allow the
// double-underscore separator used by MCP-sourced tools
// (mcp__{server}__{tool}). Hyphens are allowed because many public MCP server
// names use them.
var mcpSafeNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+(?:__[a-zA-Z0-9_-]+)*$`)

// RegisteredTool binds the executable runtime to the resolved definition
// snapshot used by model compilation and audits. Origin and Quality are kept
// separate so a trusted builtin is not mistaken for a complete contract.
type RegisteredTool struct {
	Runtime             Tool
	Definition          ToolDefinition
	Origin              ToolSource
	Quality             DefinitionQuality
	Warnings            []string
	PreserveInputObject bool
	// RegistrationID changes whenever the executable/definition bound to a
	// canonical name is replaced. Run snapshots pin this identity and Runtime.
	RegistrationID string
	Provider       ProviderManifest
}

func RegisterTool(runtime Tool) error {
	return RegisterToolWithMode(runtime, DefinitionValidationCompat)
}

// RegisterToolWithMode registers a tool under an explicit definition
// validation gate. The default RegisterTool remains compatibility-safe for
// legacy and externally authored tools; first-party builtins register through
// the explicit strict_builtin gate.
func RegisterToolWithMode(runtime Tool, mode DefinitionValidationMode) error {
	if err := ValidateToolWithMode(runtime, mode); err != nil {
		return err
	}
	report := ResolveDefinitionReport(runtime)

	mu.Lock()
	defer mu.Unlock()
	if _, exists := toolRegistry[runtime.Name()]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateTool, runtime.Name())
	}
	version := nextRegistryVersionLocked()
	toolRegistry[runtime.Name()] = registeredToolFromResolution(runtime, report, version)
	return nil
}

// RegisterToolForce registers a tool, replacing any existing registration
// with the same name. Used by the MCP manager when refreshing a server's
// tool list and by tests that re-create tool instances.
func RegisterToolForce(t Tool) error {
	return RegisterToolForceWithMode(t, DefinitionValidationCompat)
}

// RegisterToolForceWithMode is the validation-mode variant of
// RegisterToolForce. It intentionally preserves replacement semantics.
func RegisterToolForceWithMode(t Tool, mode DefinitionValidationMode) error {
	if err := ValidateToolWithMode(t, mode); err != nil {
		return err
	}
	report := ResolveDefinitionReport(t)
	mu.Lock()
	defer mu.Unlock()
	version := nextRegistryVersionLocked()
	toolRegistry[t.Name()] = registeredToolFromResolution(t, report, version)
	return nil
}

func MustRegisterTool(tool Tool) {
	if err := RegisterTool(tool); err != nil {
		panic(err)
	}
}

// MustRegisterToolWithMode is intended for fully migrated builtin packages and
// tests that want registration itself to enforce the v2 contract.
func MustRegisterToolWithMode(runtime Tool, mode DefinitionValidationMode) {
	if err := RegisterToolWithMode(runtime, mode); err != nil {
		panic(err)
	}
}

// UnregisterTool removes a tool from the registry by name. It is a no-op
// when the tool does not exist. Used by the MCP manager when a server
// disconnects or is disabled.
func UnregisterTool(name string) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := toolRegistry[name]; !exists {
		return
	}
	delete(toolRegistry, name)
	nextRegistryVersionLocked()
}

// UnregisterToolsByPrefix removes every tool whose name starts with the
// given prefix. Returns the count of removed tools. Used by the MCP
// manager to clear all tools from a single server in one call.
func UnregisterToolsByPrefix(prefix string) int {
	mu.Lock()
	defer mu.Unlock()
	removed := 0
	for name := range toolRegistry {
		if startsWith(name, prefix) {
			delete(toolRegistry, name)
			removed++
		}
	}
	if removed > 0 {
		nextRegistryVersionLocked()
	}
	return removed
}

// ListToolNames returns the names of all currently registered tools, sorted.
// Used by tests and by the /v1/tools endpoint.
func ListToolNames() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(toolRegistry))
	for name := range toolRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func startsWith(name, prefix string) bool {
	if len(prefix) > len(name) {
		return false
	}
	return name[:len(prefix)] == prefix
}

func ValidateTool(tool Tool) error {
	return ValidateToolWithMode(tool, DefinitionValidationCompat)
}

// isValidToolName accepts builtin-style names and namespaced
// mcp__{server}__{tool} forms.
func isValidToolName(name string) bool {
	if name == "" {
		return false
	}
	return mcpSafeNamePattern.MatchString(name)
}

func GetTool(name string) (Tool, error) {
	mu.RLock()
	defer mu.RUnlock()
	registered, exists := toolRegistry[name]
	if !exists {
		return nil, ErrToolNotFound
	}
	return registered.Runtime, nil
}

// GetRegisteredTool returns an immutable snapshot of the registry contract.
func GetRegisteredTool(name string) (RegisteredTool, error) {
	mu.RLock()
	defer mu.RUnlock()
	registered, exists := toolRegistry[name]
	if !exists {
		return RegisteredTool{}, ErrToolNotFound
	}
	return cloneRegisteredTool(registered), nil
}

func ListTools() []Tool {
	mu.RLock()
	defer mu.RUnlock()
	tools := make([]Tool, 0, len(toolRegistry))
	for _, registered := range toolRegistry {
		tools = append(tools, registered.Runtime)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})
	return tools
}

// ListRegisteredTools returns registry snapshots sorted by canonical name.
func ListRegisteredTools() []RegisteredTool {
	mu.RLock()
	defer mu.RUnlock()
	registered := make([]RegisteredTool, 0, len(toolRegistry))
	for _, entry := range toolRegistry {
		registered = append(registered, cloneRegisteredTool(entry))
	}
	sort.Slice(registered, func(i, j int) bool {
		return registered[i].Definition.Name < registered[j].Definition.Name
	})
	return registered
}

func GetToolsDescription() string {
	registered := ListRegisteredTools()
	if len(registered) == 0 {
		return "No tools available."
	}

	var desc string
	for _, entry := range registered {
		desc += fmt.Sprintf("- %s: %s\n", entry.Definition.Name, ModelDescription(entry.Definition))
		requiredNames := make(map[string]bool, len(entry.Definition.InputSchema.Required))
		for _, name := range entry.Definition.InputSchema.Required {
			requiredNames[name] = true
		}
		parameterNames := make([]string, 0, len(entry.Definition.InputSchema.Properties))
		for name := range entry.Definition.InputSchema.Properties {
			parameterNames = append(parameterNames, name)
		}
		sort.Strings(parameterNames)
		for _, name := range parameterNames {
			param := entry.Definition.InputSchema.Properties[name]
			required := ""
			if requiredNames[name] {
				required = " (required)"
			}
			paramType := param.Type
			if paramType == "" && len(param.OneOf) > 0 {
				paramType = "oneOf"
			}
			desc += fmt.Sprintf("  - %s (%s)%s: %s\n", name, paramType, required, param.Description)
		}
	}
	return desc
}

func registeredToolFromResolution(runtime Tool, report DefinitionResolution, version uint64) RegisteredTool {
	_, hasRecursiveSchema := runtime.(InputSchemaProvider)
	preserveInputObject := runtime.Definition().SchemaVersion == ToolDefinitionSchemaVersion || hasRecursiveSchema
	if legacy, ok := runtime.(LegacySingleValueInput); ok && legacy.UnwrapSingleParameterInput() {
		preserveInputObject = false
	}
	provider := resolveProviderManifest(runtime, report.Definition, report.Quality, report.Warnings)
	return RegisteredTool{
		Runtime:             runtime,
		Definition:          cloneToolDefinition(report.Definition),
		Origin:              report.Origin,
		Quality:             report.Quality,
		Warnings:            append([]string(nil), report.Warnings...),
		PreserveInputObject: preserveInputObject,
		RegistrationID:      fmt.Sprintf("toolreg_%016x", version),
		Provider:            provider,
	}
}

func cloneRegisteredTool(registered RegisteredTool) RegisteredTool {
	registered.Definition = cloneToolDefinition(registered.Definition)
	registered.Warnings = append([]string(nil), registered.Warnings...)
	registered.Provider = cloneProviderManifest(registered.Provider)
	return registered
}

func nextRegistryVersionLocked() uint64 {
	registryVersion++
	return registryVersion
}

// RegistryVersion reports the current monotonic catalog generation.
func RegistryVersion() uint64 {
	mu.RLock()
	defer mu.RUnlock()
	return registryVersion
}

func ExecuteTool(ctx context.Context, toolName string, input string) (string, error) {
	tool, err := GetTool(toolName)
	if err != nil {
		return "", err
	}
	return tool.Execute(ctx, input)
}

func isJSONSchemaType(value string) bool {
	switch value {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	default:
		return false
	}
}
