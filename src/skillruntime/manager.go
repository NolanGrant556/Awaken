package skillruntime

import (
	"sync"

	"awaken/src/skills"
	"awaken/src/tool"
)

// Manager owns the one model-facing skill Catalog tool plus package resource
// and install utilities. Registry changes refresh the Catalog definition; they
// never create one tool schema per Skill.
type Manager struct {
	mu          sync.Mutex
	registry    *skills.SkillRegistry
	inlineExec  *InlineExecutor
	activator   *Activator
	logger      Logger
	catalogTool *CatalogTool
	// registryFingerprint coalesces asynchronous callbacks that observe the
	// same final Skill registry state.
	registryFingerprint string
	closed              bool
}

func NewManager(reg *skills.SkillRegistry, logger Logger) *Manager {
	if logger == nil {
		logger = noopLogger{}
	}
	m := &Manager{
		registry:   reg,
		inlineExec: &InlineExecutor{Logger: logger},
		activator:  NewActivator(reg, logger),
		logger:     logger,
	}
	m.catalogTool = &CatalogTool{Registry: reg, Activator: m.activator, Executor: m.inlineExec}
	if reg != nil {
		m.registryFingerprint = reg.ToolStateFingerprint()
		reg.SetOnChange(m.refreshCatalogBinding)
	}
	if err := tool.RegisterToolForceWithMode(&ResourceReadTool{registry: reg}, tool.DefinitionValidationStrictAll); err != nil {
		logger.Warnf("failed to register skill resource reader: %v", err)
	}
	if err := tool.RegisterToolForceWithMode(&ResourceListTool{registry: reg}, tool.DefinitionValidationStrictAll); err != nil {
		logger.Warnf("failed to register skill resource lister: %v", err)
	}
	if err := tool.RegisterToolForceWithMode(&SkillInstallTool{registry: reg}, tool.DefinitionValidationStrictBuiltin); err != nil {
		logger.Warnf("failed to register skill installer: %v", err)
	}
	if err := tool.RegisterToolForceWithMode(&SkillPackageExportTool{registry: reg}, tool.DefinitionValidationStrictBuiltin); err != nil {
		logger.Warnf("failed to register skill package exporter: %v", err)
	}
	m.refreshTools()
	return m
}

func (m *Manager) Activator() *Activator {
	if m == nil {
		return nil
	}
	return m.activator
}

func (m *Manager) CatalogTool() *CatalogTool {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.catalogTool
}

func (m *Manager) Catalog(contextWindowTokens int) CatalogSnapshot {
	if m == nil || m.registry == nil {
		return CompileCatalog(nil, contextWindowTokens)
	}
	return CompileCatalog(m.registry.ListModelInvocable(), contextWindowTokens)
}

func (m *Manager) SetForkStarter(starter ChildAgentStarter) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.catalogTool == nil {
		return
	}
	next := *m.catalogTool
	next.ForkStarter = starter
	m.catalogTool = &next
	m.registerCatalogToolLocked()
}

func (m *Manager) SetContextWindowProvider(provider func() int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.catalogTool == nil {
		return
	}
	next := *m.catalogTool
	next.ContextTokens = provider
	m.catalogTool = &next
	m.registerCatalogToolLocked()
}

func (m *Manager) SetOnActivated(fn func(plan *SkillActivationPlan)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.catalogTool == nil {
		return
	}
	next := *m.catalogTool
	next.OnActivated = fn
	m.catalogTool = &next
	m.registerCatalogToolLocked()
}

func (m *Manager) RefreshTools() { m.refreshTools() }

func (m *Manager) refreshTools() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.catalogTool == nil {
		return
	}
	m.registerCatalogToolLocked()
}

// refreshCatalogBinding creates a new runtime identity only after the Skill
// registry reports a real model-visible catalog or invocation binding change.
// Old run snapshots therefore retain their previous CatalogTool instance,
// while repeated no-diff workspace refreshes never reach tool registration.
func (m *Manager) refreshCatalogBinding() {
	if m == nil {
		return
	}
	fingerprint := ""
	if m.registry != nil {
		fingerprint = m.registry.ToolStateFingerprint()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.catalogTool == nil {
		return
	}
	if fingerprint == m.registryFingerprint {
		return
	}
	m.registryFingerprint = fingerprint
	next := *m.catalogTool
	m.catalogTool = &next
	m.registerCatalogToolLocked()
}

func (m *Manager) registerCatalogToolLocked() {
	if _, err := tool.RegisterToolForceIfChanged(m.catalogTool); err != nil {
		m.logger.Warnf("failed to register unified skill catalog tool: %v", err)
	}
}

func (m *Manager) Shutdown() {
	if m == nil {
		return
	}
	if m.registry != nil {
		m.registry.SetOnChange(nil)
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.mu.Unlock()
	tool.UnregisterTool(CatalogToolName)
	tool.UnregisterTool(ResourceListToolName)
	tool.UnregisterTool(ResourceReadToolName)
	tool.UnregisterTool(SkillInstallToolName)
}
