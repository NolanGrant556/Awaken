package mcpclient

import (
	"fmt"
	"sort"
	"sync"

	"awaken/src/tool"
)

var ErrDuplicateServer = fmt.Errorf("%w: duplicate server", ErrInvalidConfig)

type Client interface {
	RegisterServer(descriptor ServerDescriptor) error
	ListServers() []ServerState
	DiscoverTools() []DiscoveredTool
	ToolDefinitions() []tool.ToolDefinition
	Discovery() []DiscoveryResult
	SetServerState(name string, state ServerState) error
}

type Registry interface {
	Client
}

type StaticRegistry struct {
	mu      sync.RWMutex
	servers map[string]ServerDescriptor
	states  map[string]ServerState
}

func NewRegistry() *StaticRegistry {
	return &StaticRegistry{
		servers: make(map[string]ServerDescriptor),
		states:  make(map[string]ServerState),
	}
}

func (r *StaticRegistry) RegisterServer(descriptor ServerDescriptor) error {
	if err := ValidateServerConfig(descriptor.Config); err != nil {
		return err
	}

	descriptor = sanitizeDescriptor(descriptor)
	name := descriptor.Config.Name
	state := descriptor.State
	if state.Status == "" {
		state = defaultStateForDescriptor(descriptor)
	}
	state.Name = name
	state.Transport = descriptor.Config.Transport
	state.Message = capStatusMessage(RedactConfigSecrets(state.Message, descriptor.Config))
	if len(descriptor.Tools) > 0 {
		descriptor.Capabilities.Tools = true
	}
	if len(descriptor.Resources) > 0 {
		descriptor.Capabilities.Resources = true
	}
	state.Capabilities = descriptor.Capabilities

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.servers[name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateServer, name)
	}
	r.servers[name] = descriptor
	r.states[name] = state
	return nil
}

func (r *StaticRegistry) ListServers() []ServerState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.states))
	for name := range r.states {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]ServerState, 0, len(names))
	for _, name := range names {
		result = append(result, r.states[name])
	}
	return result
}

func (r *StaticRegistry) DiscoverTools() []DiscoveredTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]DiscoveredTool, 0)
	for name, descriptor := range r.servers {
		if r.states[name].Status != StatusConnected {
			continue
		}
		for _, remote := range descriptor.Tools {
			result = append(result, discoveredToolFor(name, remote, descriptor.Config))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SafeName < result[j].SafeName
	})
	return result
}

func (r *StaticRegistry) ToolDefinitions() []tool.ToolDefinition {
	discovered := r.DiscoverTools()
	result := make([]tool.ToolDefinition, 0, len(discovered))
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range discovered {
		descriptor := r.servers[item.ServerName]
		result = append(result, ToolDefinitionForRemote(item.ServerName, item.Remote, descriptor.Config))
	}
	return result
}

func (r *StaticRegistry) Discovery() []DiscoveryResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.servers))
	for name := range r.servers {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]DiscoveryResult, 0, len(names))
	for _, name := range names {
		descriptor := r.servers[name]
		if r.states[name].Status != StatusConnected {
			continue
		}
		discovered := make([]DiscoveredTool, 0, len(descriptor.Tools))
		for _, remote := range descriptor.Tools {
			discovered = append(discovered, discoveredToolFor(name, remote, descriptor.Config))
		}
		sort.Slice(discovered, func(i, j int) bool {
			return discovered[i].SafeName < discovered[j].SafeName
		})
		result = append(result, DiscoveryResult{
			ServerName:   name,
			Tools:        discovered,
			Resources:    descriptor.Resources,
			Instructions: descriptor.Instructions,
		})
	}
	return result
}

func (r *StaticRegistry) SetServerState(name string, state ServerState) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	descriptor, exists := r.servers[name]
	if !exists {
		return fmt.Errorf("%w: server %q is not registered", ErrInvalidConfig, name)
	}
	state.Name = name
	state.Transport = descriptor.Config.Transport
	state.Capabilities = descriptor.Capabilities
	state.Message = capStatusMessage(RedactConfigSecrets(state.Message, descriptor.Config))
	r.states[name] = state
	return nil
}

func defaultStateForDescriptor(descriptor ServerDescriptor) ServerState {
	state := InitialState(descriptor.Config)
	if descriptor.Config.Disabled {
		return state
	}
	if len(descriptor.Tools) > 0 || len(descriptor.Resources) > 0 || descriptor.Capabilities.Tools || descriptor.Capabilities.Resources || descriptor.Capabilities.Prompts {
		state.Status = StatusConnected
	}
	return state
}

func discoveredToolFor(serverName string, remote RemoteTool, config ServerConfig) DiscoveredTool {
	description := CapDescription(RedactConfigSecrets(remote.Description, config))
	return DiscoveredTool{
		ServerName:       serverName,
		OriginalName:     remote.Name,
		SafeName:         BuildToolName(serverName, remote.Name),
		Description:      description,
		ParameterNameMap: ParameterNameMap(remote.InputSchema),
		Remote:           remote,
	}
}

func sanitizeDescriptor(descriptor ServerDescriptor) ServerDescriptor {
	descriptor.Config = cloneConfig(descriptor.Config)
	descriptor.Instructions = CapDescription(RedactConfigSecrets(descriptor.Instructions, descriptor.Config))
	descriptor.Tools = append([]RemoteTool(nil), descriptor.Tools...)
	for i := range descriptor.Tools {
		descriptor.Tools[i].Description = CapDescription(RedactConfigSecrets(descriptor.Tools[i].Description, descriptor.Config))
	}
	descriptor.Resources = append([]ServerResource(nil), descriptor.Resources...)
	for i := range descriptor.Resources {
		descriptor.Resources[i].Server = descriptor.Config.Name
		descriptor.Resources[i].Description = CapDescription(RedactConfigSecrets(descriptor.Resources[i].Description, descriptor.Config))
	}
	return descriptor
}

func cloneConfig(config ServerConfig) ServerConfig {
	config.Metadata = cloneStringMap(config.Metadata)
	if config.Stdio != nil {
		copy := *config.Stdio
		copy.Args = append([]string(nil), config.Stdio.Args...)
		copy.Env = cloneStringMap(config.Stdio.Env)
		config.Stdio = &copy
	}
	config.HTTP = cloneRemoteConfig(config.HTTP)
	config.SSE = cloneRemoteConfig(config.SSE)
	config.WS = cloneRemoteConfig(config.WS)
	return config
}

func cloneRemoteConfig(config *RemoteConfig) *RemoteConfig {
	if config == nil {
		return nil
	}
	copy := *config
	copy.Headers = cloneStringMap(config.Headers)
	if config.OAuth != nil {
		oauth := *config.OAuth
		copy.OAuth = &oauth
	}
	return &copy
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
