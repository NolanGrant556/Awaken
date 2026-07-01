package skills

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"

	"awaken/src/mcpclient"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxSkillsPerServer is the maximum number of bridged skills the bridge
// will register from a single MCP server. Beyond this limit, skills are
// truncated and a warning is logged.
const maxSkillsPerServer = 100

// Bridge subscribes to MCP server lifecycle events and reconciles the
// SkillRegistry to match the current set of MCP prompts and prompt-typed
// resources. It is safe for concurrent use.
type Bridge struct {
	// debounceMu protects the debounce map.
	debounceMu   sync.Mutex
	debounce     map[string]*time.Timer
	debounceDone map[string]chan struct{}

	logger Logger
}

// NewBridge creates a Bridge with the given logger.
func NewBridge(logger Logger) *Bridge {
	if logger == nil {
		logger = noopLogger{}
	}
	return &Bridge{
		debounce:     make(map[string]*time.Timer),
		debounceDone: make(map[string]chan struct{}),
		logger:       logger,
	}
}

// McpBridgeManager is the subset of mcpclient.Manager that the bridge needs.
type McpBridgeManager interface {
	Subscribe(ctx context.Context) <-chan mcpclient.ServerEvent
	Session(name string) (mcpclient.Session, error)
	Snapshot() []mcpclient.ServerSnapshot
}

// Start begins consuming MCP server events and driving the skill registry.
// It blocks until ctx is cancelled. Call it in a goroutine.
func (b *Bridge) Start(ctx context.Context, mcpManager McpBridgeManager, registry *SkillRegistry) {
	ch := mcpManager.Subscribe(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			b.handleEvent(ctx, mcpManager, registry, evt)
		}
	}
}

func (b *Bridge) handleEvent(ctx context.Context, mcpManager McpBridgeManager, registry *SkillRegistry, evt mcpclient.ServerEvent) {
	switch evt.Type {
	case mcpclient.EventConnected,
		mcpclient.EventPromptsListChanged,
		mcpclient.EventResourcesListChanged:
		b.debounceSync(ctx, mcpManager, registry, evt.ServerName)
	case mcpclient.EventDisconnected,
		mcpclient.EventDisabled:
		// Remove all bridged skills for this server immediately.
		prefix := "mcp__" + evt.ServerName + "__"
		registry.UnregisterSkillsByPrefix(prefix)
		registry.SetMcpServerAvailable(evt.ServerName, false)
		b.logger.Warnf("bridge: removed skills for server %q (event=%s)", evt.ServerName, evt.Type)
	}
}

func (b *Bridge) debounceSync(ctx context.Context, mcpManager McpBridgeManager, registry *SkillRegistry, serverName string) {
	b.debounceMu.Lock()

	// Cancel any pending timer for this server.
	if t, ok := b.debounce[serverName]; ok {
		t.Stop()
	}
	if done, ok := b.debounceDone[serverName]; ok {
		close(done)
	}

	done := make(chan struct{})
	b.debounceDone[serverName] = done
	timer := time.AfterFunc(300*time.Millisecond, func() {
		defer close(done)
		b.syncForServer(ctx, mcpManager, registry, serverName)
	})
	b.debounce[serverName] = timer
	b.debounceMu.Unlock()
}

// syncForServer queries the server's prompts/list and resources/list and
// idempotently reconciles the SkillRegistry.
func (b *Bridge) syncForServer(ctx context.Context, mcpManager McpBridgeManager, registry *SkillRegistry, serverName string) {
	session, err := mcpManager.Session(serverName)
	if err != nil {
		b.logger.Warnf("bridge: cannot sync skills for %q: %v", serverName, err)
		return
	}

	// Determine the source connector ID for this server.
	var sourceConnectorID string
	for _, snap := range mcpManager.Snapshot() {
		if snap.Name == serverName {
			if snap.SourceConnectorID != nil {
				sourceConnectorID = *snap.SourceConnectorID
			}
			break
		}
	}

	// Collect all bridged skills.
	var bridged []*SkillDefinition

	// Fetch prompts.
	prompts, err := session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		b.logger.Warnf("bridge: failed to list prompts for %q: %v", serverName, err)
	} else {
		for _, prompt := range prompts.Prompts {
			// Apply cap.
			if len(bridged) >= maxSkillsPerServer {
				b.logger.Warnf("bridge: server %q has more than %d prompts; truncating", serverName, maxSkillsPerServer)
				break
			}
			skill := promptToSkill(serverName, prompt, sourceConnectorID)
			bridged = append(bridged, skill)
		}
	}

	// Fetch resources.
	resources, err := session.ListResources(ctx, &mcp.ListResourcesParams{})
	if err != nil {
		b.logger.Warnf("bridge: failed to list resources for %q: %v", serverName, err)
	} else {
		for _, res := range resources.Resources {
			if res.MIMEType != "text/x-prompt" {
				// Only prompt-typed resources are bridged.
				continue
			}
			if len(bridged) >= maxSkillsPerServer {
				b.logger.Warnf("bridge: server %q has more than %d skills (prompts+resources); truncating", serverName, maxSkillsPerServer)
				break
			}
			skill := resourceToSkill(serverName, res, sourceConnectorID)
			bridged = append(bridged, skill)
		}
	}

	// Remove all existing skills for this server, then re-register the new set.
	prefix := "mcp__" + serverName + "__"
	registry.UnregisterSkillsByPrefix(prefix)

	if len(bridged) == 0 {
		registry.SetMcpServerAvailable(serverName, false)
		return
	}

	for _, skill := range bridged {
		registry.RegisterSkill(skill)
	}

	// Mark the server as available so skills that depend on it are unlocked.
	registry.SetMcpServerAvailable(serverName, true)

	b.logger.Debugf("bridge: synced %d skills for server %q", len(bridged), serverName)
}

// promptToSkill converts an MCP Prompt to a SkillDefinition.
func promptToSkill(serverName string, prompt *mcp.Prompt, sourceConnectorID string) *SkillDefinition {
	name := toKebab(prompt.Name)
	skillID := "mcp__" + serverName + "__" + name

	description := prompt.Description
	if description == "" {
		description = prompt.Title
	}

	var args []SkillArgument
	for _, arg := range prompt.Arguments {
		args = append(args, SkillArgument{
			Name:        arg.Name,
			Type:        "string",
			Description: arg.Description,
			Required:    arg.Required,
		})
	}

	var connectors []string
	if sourceConnectorID != "" {
		connectors = []string{sourceConnectorID}
	}

	return &SkillDefinition{
		ID:                 skillID,
		Name:               name,
		DisplayName:        prompt.Title,
		Description:        description,
		WhenToUse:          description,
		Arguments:          args,
		AllowedTools:       nil,
		Context:            ContextInline,
		RequiredMcpServers: []string{serverName},
		RequiredConnectors: connectors,
		Source:             SourceMcp,
		State:              StateAvailable,
		McpBinding: &McpBinding{
			ServerName: serverName,
			PromptName: prompt.Name,
			Kind:       "prompt",
		},
	}
}

// resourceToSkill converts an MCP Resource with mimeType "text/x-prompt" to a SkillDefinition.
func resourceToSkill(serverName string, res *mcp.Resource, sourceConnectorID string) *SkillDefinition {
	slug := resourceSlug(res.URI)
	name := toKebab(slug)
	skillID := "mcp__" + serverName + "__" + name

	description := res.Description
	if description == "" {
		description = res.Title
	}
	if description == "" {
		description = res.Name
	}

	var connectors []string
	if sourceConnectorID != "" {
		connectors = []string{sourceConnectorID}
	}

	return &SkillDefinition{
		ID:                 skillID,
		Name:               name,
		DisplayName:        res.Title,
		Description:        description,
		WhenToUse:          description,
		AllowedTools:       nil,
		Context:            ContextInline,
		RequiredMcpServers: []string{serverName},
		RequiredConnectors: connectors,
		Source:             SourceMcp,
		State:              StateAvailable,
		McpBinding: &McpBinding{
			ServerName: serverName,
			PromptName: slug,
			Kind:       "resource",
		},
	}
}

// toKebab normalizes a string to kebab-case.
func toKebab(s string) string {
	var result strings.Builder
	for i, r := range s {
		if r == ' ' || r == '_' || r == '-' {
			if result.Len() > 0 && result.String()[result.Len()-1] != '-' {
				result.WriteRune('-')
			}
			continue
		}
		if unicode.IsUpper(r) {
			if i > 0 && result.Len() > 0 && result.String()[result.Len()-1] != '-' {
				result.WriteRune('-')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	res := result.String()
	// Trim leading/trailing dashes and collapse multiple dashes.
	res = strings.Trim(res, "-")
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	if res == "" {
		return "unnamed"
	}
	return res
}

// resourceSlug extracts the last path segment from a resource URI.
func resourceSlug(uri string) string {
	// Strip scheme.
	idx := strings.Index(uri, "://")
	if idx >= 0 {
		uri = uri[idx+3:]
	}
	// Take the last path segment.
	if idx = strings.LastIndex(uri, "/"); idx >= 0 {
		uri = uri[idx+1:]
	}
	// Remove query string and fragment.
	if idx = strings.IndexAny(uri, "?#"); idx >= 0 {
		uri = uri[:idx]
	}
	if uri == "" {
		uri = "unnamed"
	}
	return uri
}

// Ensure unicode is used.
var _ = unicode.IsUpper