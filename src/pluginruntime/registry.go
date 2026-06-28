package pluginruntime

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	ErrInvalidPlugin       = errors.New("invalid plugin")
	ErrPluginCollision     = errors.New("plugin key collision")
	ErrInvalidRegistration = errors.New("invalid plugin registration")
	ErrToolNotFound        = errors.New("plugin tool not found")
)

var (
	safeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	safeKeyPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*(/[A-Za-z0-9][A-Za-z0-9_-]*)*$`)
)

type RegistryOptions struct {
	EnabledPluginKeys  []string
	DisabledPluginKeys []string
}

type Option func(*RegistryOptions)

func WithEnabledPlugins(keys ...string) Option {
	return func(options *RegistryOptions) {
		options.EnabledPluginKeys = normalizedKeyList(keys)
	}
}

func WithDisabledPlugins(keys ...string) Option {
	return func(options *RegistryOptions) {
		options.DisabledPluginKeys = normalizedKeyList(keys)
	}
}

type Registry struct {
	options  RegistryOptions
	plugins  map[string]*LoadedPlugin
	shadowed []*LoadedPlugin
	sequence int
}

func NewRegistry(options ...Option) *Registry {
	registryOptions := RegistryOptions{}
	for _, option := range options {
		option(&registryOptions)
	}
	return &Registry{
		options: registryOptions,
		plugins: make(map[string]*LoadedPlugin),
	}
}

func (m PluginManifest) RegistryKey() string {
	if strings.TrimSpace(m.Key) != "" {
		return strings.TrimSpace(m.Key)
	}
	return strings.TrimSpace(m.Name)
}

func (r *Registry) RegisterPlugin(manifest PluginManifest, register func(*RegistrationContext) error) (*LoadedPlugin, error) {
	manifest, err := normalizeManifest(manifest)
	if err != nil {
		return nil, err
	}

	key := manifest.RegistryKey()
	loaded := &LoadedPlugin{
		Manifest: manifest,
		Sequence: r.nextSequence(),
	}

	if r.isDisabled(manifest) {
		loaded.Error = "disabled by policy"
		r.plugins[key] = loaded
		return copyLoadedPlugin(loaded), nil
	}
	if !r.isAllowed(manifest) {
		loaded.Error = "not enabled by allowlist"
		r.plugins[key] = loaded
		return copyLoadedPlugin(loaded), nil
	}

	if existing, ok := r.plugins[key]; ok && existing.Enabled {
		switch compareSourcePriority(manifest.Source, existing.Manifest.Source) {
		case -1:
			loaded.Error = fmt.Sprintf("shadowed by %s plugin %q", existing.Manifest.Source, key)
			r.shadowed = append(r.shadowed, cloneLoadedPlugin(loaded))
			return copyLoadedPlugin(loaded), nil
		case 0:
			return nil, fmt.Errorf("%w: %q already registered from %s", ErrPluginCollision, key, existing.Manifest.Source)
		case 1:
			shadowed := cloneLoadedPlugin(existing)
			shadowed.Enabled = false
			shadowed.Error = fmt.Sprintf("shadowed by %s plugin %q", manifest.Source, key)
			r.shadowed = append(r.shadowed, shadowed)
		}
	}

	registrationContext := newRegistrationContext(manifest)
	if register != nil {
		if err := register(registrationContext); err != nil {
			loaded.Error = err.Error()
			r.plugins[key] = loaded
			return copyLoadedPlugin(loaded), err
		}
	}

	loaded.Registration = registrationContext.registration
	loaded.Enabled = true
	r.plugins[key] = loaded
	return copyLoadedPlugin(loaded), nil
}

func (r *Registry) LoadedPlugin(keyOrName string) (LoadedPlugin, bool) {
	keyOrName = strings.TrimSpace(keyOrName)
	if keyOrName == "" {
		return LoadedPlugin{}, false
	}
	if plugin, ok := r.plugins[keyOrName]; ok {
		return *copyLoadedPlugin(plugin), true
	}
	for _, plugin := range r.plugins {
		if plugin.Manifest.Name == keyOrName {
			return *copyLoadedPlugin(plugin), true
		}
	}
	return LoadedPlugin{}, false
}

func (r *Registry) ListEnabledPlugins() []LoadedPlugin {
	active := r.activePlugins()
	out := make([]LoadedPlugin, 0, len(active))
	for _, plugin := range active {
		out = append(out, *copyLoadedPlugin(plugin))
	}
	return out
}

func (r *Registry) ListShadowedPlugins() []LoadedPlugin {
	out := make([]LoadedPlugin, 0, len(r.shadowed))
	for _, plugin := range r.shadowed {
		out = append(out, *copyLoadedPlugin(plugin))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Manifest.RegistryKey() == out[j].Manifest.RegistryKey() {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].Manifest.RegistryKey() < out[j].Manifest.RegistryKey()
	})
	return out
}

func (r *Registry) ListEnabledCapabilities() []ExtensionCapability {
	var capabilities []ExtensionCapability
	for _, plugin := range r.activePlugins() {
		ref := pluginRef(plugin)
		for _, hook := range plugin.Registration.Hooks {
			capabilities = append(capabilities, ExtensionCapability{
				Plugin:      ref,
				Kind:        CapabilityHook,
				Name:        string(hook.Event),
				Description: hook.Description,
			})
		}
		for _, command := range plugin.Registration.SlashCommands {
			capabilities = append(capabilities, ExtensionCapability{
				Plugin:      ref,
				Kind:        CapabilitySlashCommand,
				Name:        command.Name,
				Description: command.Description,
			})
		}
		for _, command := range plugin.Registration.CLICommands {
			capabilities = append(capabilities, ExtensionCapability{
				Plugin:      ref,
				Kind:        CapabilityCLICommand,
				Name:        command.Name,
				Description: command.Description,
			})
		}
		for _, tool := range plugin.Registration.Tools {
			capabilities = append(capabilities, ExtensionCapability{
				Plugin:      ref,
				Kind:        CapabilityTool,
				Name:        tool.Name,
				Description: tool.Description,
			})
		}
	}
	return capabilities
}

func (r *Registry) SlashCommand(name string) (CommandRegistration, bool) {
	name, err := normalizeCommandName(name)
	if err != nil {
		return CommandRegistration{}, false
	}
	for _, plugin := range r.activePlugins() {
		for _, command := range plugin.Registration.SlashCommands {
			if command.Name == name {
				return command, true
			}
		}
	}
	return CommandRegistration{}, false
}

func (r *Registry) CLICommand(name string) (CommandRegistration, bool) {
	name, err := normalizeCommandName(name)
	if err != nil {
		return CommandRegistration{}, false
	}
	for _, plugin := range r.activePlugins() {
		for _, command := range plugin.Registration.CLICommands {
			if command.Name == name {
				return command, true
			}
		}
	}
	return CommandRegistration{}, false
}

func (r *Registry) Tool(name string) (ToolRegistration, bool) {
	name = strings.TrimSpace(name)
	for _, plugin := range r.activePlugins() {
		for _, tool := range plugin.Registration.Tools {
			if tool.Name == name {
				return tool, true
			}
		}
	}
	return ToolRegistration{}, false
}

func (r *Registry) DispatchPreToolCall(ctx context.Context, call ToolCallEvent) (PreToolCallDecision, HookReport) {
	report := HookReport{}
	for _, binding := range r.hooksFor(HookPreToolCall) {
		result, hookErr := invokeHook(ctx, binding, HookInvocation{
			Event:    HookPreToolCall,
			Plugin:   pluginRef(binding.plugin),
			ToolCall: call,
		})
		if hookErr != nil {
			report.Errors = append(report.Errors, *hookErr)
			continue
		}
		if result.PreToolCall == nil || result.PreToolCall.Action != PreToolCallBlock {
			continue
		}
		decision := *result.PreToolCall
		decision.Action = PreToolCallBlock
		if strings.TrimSpace(decision.Message) == "" {
			decision.Message = "blocked by plugin"
		}
		decision.PluginName = binding.plugin.Manifest.Name
		decision.PluginKey = binding.plugin.Manifest.RegistryKey()
		return decision, report
	}
	return PreToolCallDecision{Action: PreToolCallAllow}, report
}

func (r *Registry) DispatchPostToolCall(ctx context.Context, result ToolResultEvent) HookReport {
	report := HookReport{}
	for _, binding := range r.hooksFor(HookPostToolCall) {
		_, hookErr := invokeHook(ctx, binding, HookInvocation{
			Event:      HookPostToolCall,
			Plugin:     pluginRef(binding.plugin),
			ToolCall:   result.Call,
			ToolResult: result,
		})
		if hookErr != nil {
			report.Errors = append(report.Errors, *hookErr)
		}
	}
	return report
}

func (r *Registry) DispatchTransformToolResult(ctx context.Context, result ToolResultEvent) (string, HookReport) {
	report := HookReport{}
	current := result.Result
	for _, binding := range r.hooksFor(HookTransformToolResult) {
		currentResult := result
		currentResult.Result = current
		hookResult, hookErr := invokeHook(ctx, binding, HookInvocation{
			Event:      HookTransformToolResult,
			Plugin:     pluginRef(binding.plugin),
			ToolCall:   currentResult.Call,
			ToolResult: currentResult,
		})
		if hookErr != nil {
			report.Errors = append(report.Errors, *hookErr)
			continue
		}
		if hookResult.ToolResult != nil {
			current = *hookResult.ToolResult
		}
	}
	return current, report
}

func (r *Registry) DispatchToolResult(ctx context.Context, result ToolResultEvent) (string, HookReport) {
	postReport := r.DispatchPostToolCall(ctx, result)
	transformed, transformReport := r.DispatchTransformToolResult(ctx, result)
	return transformed, mergeReports(postReport, transformReport)
}

func (r *Registry) DispatchTransformTerminalOutput(ctx context.Context, output TerminalOutputEvent) (string, HookReport) {
	report := HookReport{}
	current := output.Output
	for _, binding := range r.hooksFor(HookTransformTerminalOutput) {
		currentOutput := output
		currentOutput.Output = current
		hookResult, hookErr := invokeHook(ctx, binding, HookInvocation{
			Event:          HookTransformTerminalOutput,
			Plugin:         pluginRef(binding.plugin),
			ToolCall:       currentOutput.Call,
			TerminalOutput: currentOutput,
		})
		if hookErr != nil {
			report.Errors = append(report.Errors, *hookErr)
			continue
		}
		if hookResult.TerminalOutput != nil {
			current = *hookResult.TerminalOutput
		}
	}
	return current, report
}

func (r *Registry) DispatchPreLLMCall(ctx context.Context, call LLMCallEvent) ([]ContextInjection, HookReport) {
	return r.dispatchLLMContextHooks(ctx, HookPreLLMCall, call, LLMResponseEvent{})
}

func (r *Registry) DispatchPostLLMCall(ctx context.Context, call LLMCallEvent, response LLMResponseEvent) ([]ContextInjection, HookReport) {
	return r.dispatchLLMContextHooks(ctx, HookPostLLMCall, call, response)
}

func (r *Registry) DispatchSessionLifecycle(ctx context.Context, event HookEvent, session SessionEvent) HookReport {
	report := HookReport{}
	if !isSessionHook(event) {
		report.Errors = append(report.Errors, HookError{Event: event, Message: "invalid session lifecycle hook"})
		return report
	}
	for _, binding := range r.hooksFor(event) {
		_, hookErr := invokeHook(ctx, binding, HookInvocation{
			Event:   event,
			Plugin:  pluginRef(binding.plugin),
			Session: session,
		})
		if hookErr != nil {
			report.Errors = append(report.Errors, *hookErr)
		}
	}
	return report
}

type ToolBlockedError struct {
	Decision PreToolCallDecision
}

func (e ToolBlockedError) Error() string {
	if e.Decision.Message != "" {
		return e.Decision.Message
	}
	return "tool call blocked by plugin"
}

func (r *Registry) DispatchTool(ctx context.Context, call ToolCallEvent) (string, HookReport, error) {
	decision, preReport := r.DispatchPreToolCall(ctx, call)
	if !decision.Allowed() {
		return "", preReport, ToolBlockedError{Decision: decision}
	}

	tool, ok := r.Tool(call.Name)
	if !ok {
		return "", preReport, fmt.Errorf("%w: %s", ErrToolNotFound, call.Name)
	}
	if tool.Handler == nil {
		return "", preReport, fmt.Errorf("%w: %s has no handler", ErrInvalidRegistration, call.Name)
	}

	output, err := tool.Handler(ctx, call)
	transformed, toolReport := r.DispatchToolResult(ctx, ToolResultEvent{
		Call:    call,
		Result:  output,
		IsError: err != nil,
	})
	return transformed, mergeReports(preReport, toolReport), err
}

func (r *Registry) dispatchLLMContextHooks(ctx context.Context, event HookEvent, call LLMCallEvent, response LLMResponseEvent) ([]ContextInjection, HookReport) {
	report := HookReport{}
	var injections []ContextInjection
	for _, binding := range r.hooksFor(event) {
		hookResult, hookErr := invokeHook(ctx, binding, HookInvocation{
			Event:       event,
			Plugin:      pluginRef(binding.plugin),
			LLMCall:     call,
			LLMResponse: response,
		})
		if hookErr != nil {
			report.Errors = append(report.Errors, *hookErr)
			continue
		}
		for _, injection := range hookResult.ContextInjections {
			injections = append(injections, normalizeContextInjection(injection, binding.plugin))
		}
	}
	return injections, report
}

type hookBinding struct {
	plugin *LoadedPlugin
	hook   HookRegistration
}

func (r *Registry) hooksFor(event HookEvent) []hookBinding {
	var bindings []hookBinding
	for _, plugin := range r.activePlugins() {
		for _, hook := range plugin.Registration.Hooks {
			if hook.Event == event {
				bindings = append(bindings, hookBinding{plugin: plugin, hook: hook})
			}
		}
	}
	return bindings
}

func (r *Registry) activePlugins() []*LoadedPlugin {
	active := make([]*LoadedPlugin, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		if plugin.Enabled {
			active = append(active, plugin)
		}
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].Sequence == active[j].Sequence {
			return active[i].Manifest.RegistryKey() < active[j].Manifest.RegistryKey()
		}
		return active[i].Sequence < active[j].Sequence
	})
	return active
}

func (r *Registry) nextSequence() int {
	r.sequence++
	return r.sequence
}

func (r *Registry) isDisabled(manifest PluginManifest) bool {
	return keyListContains(r.options.DisabledPluginKeys, manifest)
}

func (r *Registry) isAllowed(manifest PluginManifest) bool {
	if r.options.EnabledPluginKeys == nil {
		return true
	}
	return keyListContains(r.options.EnabledPluginKeys, manifest)
}

type RegistrationContext struct {
	manifest     PluginManifest
	registration Registration
	slashNames   map[string]struct{}
	cliNames     map[string]struct{}
	toolNames    map[string]struct{}
}

func newRegistrationContext(manifest PluginManifest) *RegistrationContext {
	return &RegistrationContext{
		manifest:   manifest,
		slashNames: make(map[string]struct{}),
		cliNames:   make(map[string]struct{}),
		toolNames:  make(map[string]struct{}),
	}
}

func (c *RegistrationContext) Manifest() PluginManifest {
	return c.manifest
}

func (c *RegistrationContext) RegisterHook(event HookEvent, handler HookHandler) error {
	return c.RegisterHookWithDescription(event, "", handler)
}

func (c *RegistrationContext) RegisterHookWithDescription(event HookEvent, description string, handler HookHandler) error {
	if !isKnownHook(event) {
		return fmt.Errorf("%w: unknown hook %q", ErrInvalidRegistration, event)
	}
	if handler == nil {
		return fmt.Errorf("%w: hook %q has nil handler", ErrInvalidRegistration, event)
	}
	c.registration.Hooks = append(c.registration.Hooks, HookRegistration{
		Event:       event,
		Description: strings.TrimSpace(description),
		Handler:     handler,
	})
	return nil
}

func (c *RegistrationContext) RegisterSlashCommand(command CommandRegistration) error {
	normalized, err := normalizeCommandRegistration(command)
	if err != nil {
		return err
	}
	if err := rememberCapabilityName(c.slashNames, normalized.Name, "slash command"); err != nil {
		return err
	}
	c.registration.SlashCommands = append(c.registration.SlashCommands, normalized)
	return nil
}

func (c *RegistrationContext) RegisterCLICommand(command CommandRegistration) error {
	normalized, err := normalizeCommandRegistration(command)
	if err != nil {
		return err
	}
	if err := rememberCapabilityName(c.cliNames, normalized.Name, "CLI command"); err != nil {
		return err
	}
	c.registration.CLICommands = append(c.registration.CLICommands, normalized)
	return nil
}

func (c *RegistrationContext) RegisterTool(tool ToolRegistration) error {
	tool.Name = strings.TrimSpace(tool.Name)
	tool.Description = strings.TrimSpace(tool.Description)
	if !safeNamePattern.MatchString(tool.Name) {
		return fmt.Errorf("%w: invalid tool name %q", ErrInvalidRegistration, tool.Name)
	}
	if err := rememberCapabilityName(c.toolNames, tool.Name, "tool"); err != nil {
		return err
	}
	c.registration.Tools = append(c.registration.Tools, tool)
	return nil
}

func normalizeManifest(manifest PluginManifest) (PluginManifest, error) {
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Key = strings.TrimSpace(manifest.Key)
	manifest.Description = strings.TrimSpace(manifest.Description)
	manifest.Author = strings.TrimSpace(manifest.Author)
	manifest.Version = strings.TrimSpace(manifest.Version)
	if manifest.Source == "" {
		manifest.Source = SourceBundled
	}
	if !safeNamePattern.MatchString(manifest.Name) {
		return PluginManifest{}, fmt.Errorf("%w: plugin name %q must match %s", ErrInvalidPlugin, manifest.Name, safeNamePattern.String())
	}
	if manifest.Key == "" {
		manifest.Key = manifest.Name
	}
	if !safeKeyPattern.MatchString(manifest.Key) {
		return PluginManifest{}, fmt.Errorf("%w: plugin key %q must match %s", ErrInvalidPlugin, manifest.Key, safeKeyPattern.String())
	}
	if !isKnownSource(manifest.Source) {
		return PluginManifest{}, fmt.Errorf("%w: unknown source %q", ErrInvalidPlugin, manifest.Source)
	}
	return manifest, nil
}

func normalizeCommandRegistration(command CommandRegistration) (CommandRegistration, error) {
	name, err := normalizeCommandName(command.Name)
	if err != nil {
		return CommandRegistration{}, err
	}
	command.Name = name
	command.Description = strings.TrimSpace(command.Description)
	command.Usage = strings.TrimSpace(command.Usage)
	return command, nil
}

func normalizeCommandName(name string) (string, error) {
	clean := strings.TrimSpace(name)
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.ToLower(strings.ReplaceAll(clean, " ", "-"))
	if !safeNamePattern.MatchString(clean) {
		return "", fmt.Errorf("%w: invalid command name %q", ErrInvalidRegistration, name)
	}
	return clean, nil
}

func rememberCapabilityName(seen map[string]struct{}, name string, kind string) error {
	if _, ok := seen[name]; ok {
		return fmt.Errorf("%w: duplicate %s %q", ErrInvalidRegistration, kind, name)
	}
	seen[name] = struct{}{}
	return nil
}

func invokeHook(ctx context.Context, binding hookBinding, invocation HookInvocation) (result HookResult, hookErr *HookError) {
	defer func() {
		if recovered := recover(); recovered != nil {
			hookErr = &HookError{
				PluginName: binding.plugin.Manifest.Name,
				PluginKey:  binding.plugin.Manifest.RegistryKey(),
				Event:      binding.hook.Event,
				Message:    fmt.Sprintf("panic: %v", recovered),
			}
		}
	}()

	result, err := binding.hook.Handler(ctx, invocation)
	if err != nil {
		return HookResult{}, &HookError{
			PluginName: binding.plugin.Manifest.Name,
			PluginKey:  binding.plugin.Manifest.RegistryKey(),
			Event:      binding.hook.Event,
			Message:    err.Error(),
		}
	}
	return result, nil
}

func normalizeContextInjection(injection ContextInjection, plugin *LoadedPlugin) ContextInjection {
	if injection.PluginName == "" {
		injection.PluginName = plugin.Manifest.Name
	}
	if injection.PluginKey == "" {
		injection.PluginKey = plugin.Manifest.RegistryKey()
	}
	if strings.TrimSpace(injection.Role) == "" {
		injection.Role = "user"
	}
	return injection
}

func pluginRef(plugin *LoadedPlugin) PluginRef {
	return PluginRef{
		Name:   plugin.Manifest.Name,
		Key:    plugin.Manifest.RegistryKey(),
		Source: plugin.Manifest.Source,
	}
}

func cloneLoadedPlugin(plugin *LoadedPlugin) *LoadedPlugin {
	if plugin == nil {
		return nil
	}
	clone := *plugin
	clone.Registration.Hooks = append([]HookRegistration(nil), plugin.Registration.Hooks...)
	clone.Registration.SlashCommands = append([]CommandRegistration(nil), plugin.Registration.SlashCommands...)
	clone.Registration.CLICommands = append([]CommandRegistration(nil), plugin.Registration.CLICommands...)
	clone.Registration.Tools = append([]ToolRegistration(nil), plugin.Registration.Tools...)
	return &clone
}

func copyLoadedPlugin(plugin *LoadedPlugin) *LoadedPlugin {
	return cloneLoadedPlugin(plugin)
}

func normalizedKeyList(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		clean := strings.TrimSpace(key)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func keyListContains(keys []string, manifest PluginManifest) bool {
	for _, key := range keys {
		if key == manifest.RegistryKey() || key == manifest.Name {
			return true
		}
	}
	return false
}

func compareSourcePriority(newSource PluginSource, existingSource PluginSource) int {
	newPriority := sourcePriority(newSource)
	existingPriority := sourcePriority(existingSource)
	if newPriority < existingPriority {
		return -1
	}
	if newPriority > existingPriority {
		return 1
	}
	return 0
}

func sourcePriority(source PluginSource) int {
	switch source {
	case SourceBundled:
		return 10
	case SourceUser:
		return 20
	case SourceProject:
		return 30
	case SourceEntrypoint:
		return 40
	default:
		return 0
	}
}

func isKnownSource(source PluginSource) bool {
	switch source {
	case SourceBundled, SourceUser, SourceProject, SourceEntrypoint:
		return true
	default:
		return false
	}
}

func isKnownHook(event HookEvent) bool {
	switch event {
	case HookPreToolCall,
		HookPostToolCall,
		HookTransformToolResult,
		HookTransformTerminalOutput,
		HookPreLLMCall,
		HookPostLLMCall,
		HookOnSessionStart,
		HookOnSessionEnd,
		HookOnSessionFinalize,
		HookOnSessionReset:
		return true
	default:
		return false
	}
}

func isSessionHook(event HookEvent) bool {
	switch event {
	case HookOnSessionStart, HookOnSessionEnd, HookOnSessionFinalize, HookOnSessionReset:
		return true
	default:
		return false
	}
}

func mergeReports(reports ...HookReport) HookReport {
	merged := HookReport{}
	for _, report := range reports {
		merged.Errors = append(merged.Errors, report.Errors...)
	}
	return merged
}
