package dialect

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"awaken/src/plugins/ir"
)

const (
	awakenSetupExtension = "io.awaken.setup"
	awakenSetupSidecar   = ".awaken/setup.json"
	awakenSetupSchemaV1  = "https://awaken.dev/schemas/plugin-setup/1.0.0/setup.schema.json"
)

var setupIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type setupDocument struct {
	Schema       string                `json:"$schema,omitempty"`
	Version      int                   `json:"version"`
	Requirements []ir.SetupRequirement `json:"requirements"`
}

// appendSetupDeclarations merges native vendor declarations with the optional
// Awaken extension. It is intentionally part of dialect parsing so validation,
// installation and runtime reload all see the same immutable declaration.
func appendSetupDeclarations(root string, manifest map[string]any, manifestPath string, dialect ir.Dialect, plugin *ir.NormalizedPlugin) error {
	if plugin == nil {
		return fmt.Errorf("plugin setup target is unavailable")
	}
	var extensionRaw []byte
	if dialect == ir.DialectAgentPlugins {
		if extensions, ok := manifest["extensions"].(map[string]any); ok {
			if value, exists := extensions[awakenSetupExtension]; exists {
				extensionRaw, _ = json.Marshal(value)
			}
		}
	}
	var sidecarRaw []byte
	if existingPath(root, awakenSetupSidecar) {
		var err error
		sidecarRaw, err = safeRead(root, awakenSetupSidecar)
		if err != nil {
			return fmt.Errorf("read %s: %w", awakenSetupSidecar, err)
		}
	}
	if len(extensionRaw) > 0 && len(sidecarRaw) > 0 {
		return fmt.Errorf("plugin setup declaration conflict: use either %s or plugin.json extensions.%s", awakenSetupSidecar, awakenSetupExtension)
	}

	if raw := firstNonEmptyBytes(extensionRaw, sidecarRaw); len(raw) > 0 {
		sourcePath := awakenSetupSidecar
		if len(extensionRaw) > 0 {
			sourcePath = manifestPath + "#extensions/" + awakenSetupExtension
		}
		requirements, components, err := parseSetupDocument(raw, sourcePath, dialect, plugin.Components)
		if err != nil {
			return err
		}
		plugin.Setup = append(plugin.Setup, requirements...)
		plugin.Components = append(plugin.Components, components...)
	}

	appendNativeSetup(plugin)
	if err := finalizeSetupRequirements(plugin); err != nil {
		return err
	}
	return nil
}

func firstNonEmptyBytes(values ...[]byte) []byte {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func parseSetupDocument(raw []byte, sourcePath string, dialect ir.Dialect, existing []ir.Component) ([]ir.SetupRequirement, []ir.Component, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document setupDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, nil, fmt.Errorf("%s is not a valid setup document: %w", sourcePath, err)
	}
	if document.Version != 1 {
		return nil, nil, fmt.Errorf("%s uses unsupported setup version %d", sourcePath, document.Version)
	}
	if document.Schema != "" && document.Schema != awakenSetupSchemaV1 {
		return nil, nil, fmt.Errorf("%s uses unsupported setup schema %q", sourcePath, document.Schema)
	}
	components := append([]ir.Component(nil), existing...)
	var declaredComponents []ir.Component
	for index := range document.Requirements {
		requirement := &document.Requirements[index]
		requirement.ID = strings.TrimSpace(requirement.ID)
		if !setupIDPattern.MatchString(requirement.ID) {
			return nil, nil, fmt.Errorf("%s requirements[%d].id is invalid", sourcePath, index)
		}
		if !requirement.Kind.Valid() || requirement.Kind == ir.SetupMCPOAuth || requirement.Kind == ir.SetupAppConnector {
			return nil, nil, fmt.Errorf("%s requirement %q uses unsupported authored kind %q", sourcePath, requirement.ID, requirement.Kind)
		}
		requirement.SourceDialect = dialect
		requirement.SourcePath = sourcePath
		requirement.Raw, _ = json.Marshal(requirement)
		if requirement.Field != nil {
			if requirement.Kind != ir.SetupConfiguration && requirement.Kind != ir.SetupSecret {
				return nil, nil, fmt.Errorf("%s requirement %q may declare field only for configuration or secret", sourcePath, requirement.ID)
			}
			field := *requirement.Field
			field.Key = strings.TrimSpace(field.Key)
			field.Type = strings.TrimSpace(field.Type)
			if field.Key == "" || field.Type == "" {
				return nil, nil, fmt.Errorf("%s requirement %q field requires key and type", sourcePath, requirement.ID)
			}
			field.Secret = requirement.Kind == ir.SetupSecret
			field.Required = requirement.Required || field.Required
			requirement.Required = field.Required
			requirement.Field = &field
			fieldRaw, _ := json.Marshal(field)
			component := ir.Component{
				LogicalID: ir.LogicalID(ir.KindVariable, sourcePath, field.Key), Kind: ir.KindVariable,
				DeclaredName: field.Key, DerivedName: field.Key, DisplayName: firstNonEmpty(field.Title, field.Key), Description: field.Description,
				SourceDialect: dialect, SourceManifestPath: sourcePath, SourcePath: sourcePath,
				SourceDigest: digestBytes(fieldRaw), Mapping: ir.MappingAdapted, Validation: "valid",
				CanonicalProvenance: string(dialect), Variable: &field,
			}
			declaredComponents = append(declaredComponents, component)
			components = append(components, component)
		}
		if requirement.Kind == ir.SetupOAuthPKCE || requirement.Kind == ir.SetupOAuthDevice {
			if requirement.OAuth == nil {
				return nil, nil, fmt.Errorf("%s requirement %q requires oauth", sourcePath, requirement.ID)
			}
			if err := validateOAuthSetup(requirement.Kind, *requirement.OAuth); err != nil {
				return nil, nil, fmt.Errorf("%s requirement %q: %w", sourcePath, requirement.ID, err)
			}
		}
		if requirement.Kind == ir.SetupCLILogin && strings.TrimSpace(requirement.ProviderID) == "" {
			return nil, nil, fmt.Errorf("%s requirement %q requires providerId", sourcePath, requirement.ID)
		}
		if requirement.Kind == ir.SetupHostConnector && strings.TrimSpace(requirement.ConnectorID) == "" {
			return nil, nil, fmt.Errorf("%s requirement %q requires connectorId", sourcePath, requirement.ID)
		}
		if requirement.Kind == ir.SetupHostConnector {
			requirement.ConnectorID = strings.TrimSpace(requirement.ConnectorID)
			shortName := strings.TrimSpace(requirement.MCPServerName)
			if shortName == "" {
				shortName = requirement.ConnectorID
			}
			component := ir.Component{
				LogicalID: ir.LogicalID(ir.KindMCPServer, sourcePath, "host_connector:"+requirement.ID), Kind: ir.KindMCPServer,
				DeclaredName: shortName, DerivedName: shortName, DisplayName: firstNonEmpty(requirement.Title, shortName),
				Description: requirement.Description, SourceDialect: dialect, SourceManifestPath: sourcePath, SourcePath: sourcePath,
				SourceDigest: digestBytes(requirement.Raw), Mapping: ir.MappingAdapted, Validation: "valid",
				CanonicalProvenance: string(dialect) + ":host_connector",
				MCP: &ir.MCPSpec{
					ShortName: shortName, HostConnectorID: requirement.ConnectorID,
					SourceShape: "host_connector", SourceTransport: "in-process", Transport: "in-process", Raw: requirement.Raw,
				},
			}
			declaredComponents = append(declaredComponents, component)
			components = append(components, component)
			requirement.ComponentIDs = append(requirement.ComponentIDs, component.LogicalID)
		}
		if requirement.Kind == ir.SetupExternal {
			if err := validateSetupURL(requirement.ExternalURL); err != nil {
				return nil, nil, fmt.Errorf("%s requirement %q: %w", sourcePath, requirement.ID, err)
			}
		}
	}
	if err := validateSetupBindings(document.Requirements, components); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", sourcePath, err)
	}
	return document.Requirements, declaredComponents, nil
}

func validateOAuthSetup(kind ir.SetupKind, spec ir.OAuthSetupSpec) error {
	if strings.TrimSpace(spec.ClientID) == "" && strings.TrimSpace(spec.ClientIDVariable) == "" {
		return fmt.Errorf("oauth clientId or clientIdVariable is required")
	}
	if strings.TrimSpace(spec.ClientID) != "" && strings.TrimSpace(spec.ClientIDVariable) != "" {
		return fmt.Errorf("oauth clientId and clientIdVariable are mutually exclusive")
	}
	if kind == ir.SetupOAuthPKCE {
		if strings.TrimSpace(spec.Issuer) == "" && (strings.TrimSpace(spec.AuthorizationEndpoint) == "" || strings.TrimSpace(spec.TokenEndpoint) == "") {
			return fmt.Errorf("oauth_pkce requires issuer or authorizationEndpoint plus tokenEndpoint")
		}
		for _, endpoint := range []string{spec.Issuer, spec.AuthorizationEndpoint, spec.TokenEndpoint} {
			if strings.TrimSpace(endpoint) != "" {
				if err := validateSetupURL(endpoint); err != nil {
					return err
				}
			}
		}
	}
	if kind == ir.SetupOAuthDevice {
		if strings.TrimSpace(spec.DeviceAuthorizationEndpoint) == "" || strings.TrimSpace(spec.TokenEndpoint) == "" {
			return fmt.Errorf("oauth_device requires deviceAuthorizationEndpoint and tokenEndpoint")
		}
		for _, endpoint := range []string{spec.DeviceAuthorizationEndpoint, spec.TokenEndpoint} {
			if err := validateSetupURL(endpoint); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSetupURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("setup URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("setup URL must not contain userinfo or fragment")
	}
	if parsed.Scheme == "http" {
		host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		loopback := host == "localhost"
		if ip := net.ParseIP(host); ip != nil {
			loopback = ip.IsLoopback()
		}
		if !loopback {
			return fmt.Errorf("non-loopback setup URLs must use HTTPS")
		}
	}
	for key := range parsed.Query() {
		lower := strings.ToLower(key)
		if lower == "token" || lower == "access_token" || lower == "client_secret" || lower == "password" || lower == "api_key" {
			return fmt.Errorf("setup URL must not contain credential query parameters")
		}
	}
	return nil
}

func validateSetupBindings(requirements []ir.SetupRequirement, components []ir.Component) error {
	componentByID := make(map[string]ir.Component, len(components))
	for _, component := range components {
		componentByID[component.LogicalID] = component
	}
	for _, requirement := range requirements {
		for _, binding := range requirement.Bindings {
			component, ok := componentByID[binding.ComponentID]
			if !ok {
				return fmt.Errorf("requirement %q binding references unknown component %q", requirement.ID, binding.ComponentID)
			}
			switch binding.Target {
			case ir.SetupBindingMCPTransport:
				if component.MCP == nil || component.MCP.Transport == "stdio" {
					return fmt.Errorf("requirement %q mcp_transport binding requires a remote MCP component", requirement.ID)
				}
			case ir.SetupBindingHTTPHeader:
				if strings.TrimSpace(binding.Name) == "" || http.CanonicalHeaderKey(binding.Name) == "" || strings.ContainsAny(binding.Name, "\r\n:") {
					return fmt.Errorf("requirement %q has an invalid HTTP header binding", requirement.ID)
				}
				if component.MCP == nil && (component.Hook == nil || component.Hook.HandlerType != "http") {
					return fmt.Errorf("requirement %q http_header binding requires remote MCP or HTTP Hook", requirement.ID)
				}
			case ir.SetupBindingProcessEnv:
				if !validEnvironmentName(binding.Name) {
					return fmt.Errorf("requirement %q has an invalid process environment binding", requirement.ID)
				}
				if component.MCP == nil && (component.Hook == nil || component.Hook.HandlerType != "command") {
					return fmt.Errorf("requirement %q process_env binding requires MCP or command Hook", requirement.ID)
				}
			default:
				return fmt.Errorf("requirement %q uses unsupported binding target %q", requirement.ID, binding.Target)
			}
		}
	}
	return nil
}

func validEnvironmentName(value string) bool {
	if value == "" || strings.ContainsAny(value, "=\x00") {
		return false
	}
	for index, r := range value {
		if (r >= 'A' && r <= 'Z') || r == '_' || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func appendNativeSetup(plugin *ir.NormalizedPlugin) {
	fieldRequirements := map[string]bool{}
	for _, requirement := range plugin.Setup {
		if requirement.Field != nil {
			fieldRequirements[requirement.Field.Key] = true
		}
	}
	for _, component := range plugin.Components {
		if component.Variable != nil && !fieldRequirements[component.Variable.Key] {
			kind := ir.SetupConfiguration
			if component.Variable.Secret {
				kind = ir.SetupSecret
			}
			bindings := setupVariableBindings(plugin.Components, component.Variable.Key, component.SourceDialect)
			deps := setupVariableDependencies(plugin.Components, component.Variable.Key)
			for _, binding := range bindings {
				if !containsString(deps, binding.ComponentID) {
					deps = append(deps, binding.ComponentID)
				}
			}
			plugin.Setup = append(plugin.Setup, ir.SetupRequirement{
				ID: "variable:" + component.Variable.Key, Kind: kind,
				Title: firstNonEmpty(component.Variable.Title, component.Variable.Key), Description: component.Variable.Description,
				Required: component.Variable.Required, ComponentIDs: deps, Field: component.Variable,
				Bindings:      bindings,
				SourceDialect: component.SourceDialect, SourcePath: component.SourcePath,
			})
		}
		if component.MCP != nil && component.MCP.HostConnectorID == "" && component.MCP.Transport != "stdio" {
			plugin.Setup = append(plugin.Setup, ir.SetupRequirement{
				ID: "mcp:" + component.LogicalID, Kind: ir.SetupMCPOAuth,
				Title:       firstNonEmpty(component.DisplayName, component.MCP.ShortName) + " 授权",
				Description: "连接远程 MCP 服务所需的账号授权", Required: strings.TrimSpace(component.MCP.OAuthResource) != "",
				ComponentIDs:  []string{component.LogicalID},
				MCPServerName: component.MCP.ShortName,
				OAuth: &ir.OAuthSetupSpec{
					Resource: component.MCP.OAuthResource, ClientID: component.MCP.OAuthClientID,
					ClientMetadataURL: component.MCP.OAuthClientMetadataURL, Scopes: append([]string(nil), component.MCP.OAuthScopes...),
				},
				Bindings:      []ir.SetupBinding{{ComponentID: component.LogicalID, Target: ir.SetupBindingMCPTransport}},
				SourceDialect: component.SourceDialect, SourcePath: component.SourcePath,
			})
		}
		if component.App != nil {
			plugin.Setup = append(plugin.Setup, ir.SetupRequirement{
				ID: "app:" + component.LogicalID, Kind: ir.SetupAppConnector,
				Title:       firstNonEmpty(component.DisplayName, component.App.ConnectorID),
				Description: "该 App 引用宿主平台连接器；Awaken 当前仅识别，不运行。", Required: true,
				ComponentIDs: []string{component.LogicalID}, ConnectorID: component.App.ConnectorID,
				SourceDialect: component.SourceDialect, SourcePath: component.SourcePath,
			})
		}
	}
}

func setupVariableBindings(components []ir.Component, key string, dialect ir.Dialect) []ir.SetupBinding {
	markers := []string{"${" + key + "}", "${user_config." + key + "}", "${env:" + key + "}"}
	containsMarker := func(value string) bool {
		for _, marker := range markers {
			if strings.Contains(value, marker) {
				return true
			}
		}
		return false
	}
	var bindings []ir.SetupBinding
	for _, component := range components {
		if component.MCP != nil {
			for name, value := range component.MCP.Env {
				if containsMarker(value) {
					bindings = append(bindings, ir.SetupBinding{ComponentID: component.LogicalID, Target: ir.SetupBindingProcessEnv, Name: name})
				}
			}
			for name, value := range component.MCP.Headers {
				if containsMarker(value) {
					bindings = append(bindings, ir.SetupBinding{ComponentID: component.LogicalID, Target: ir.SetupBindingHTTPHeader, Name: name})
				}
			}
		}
		if dialect == ir.DialectClaudeCode && component.Hook != nil && (component.Hook.HandlerType == "" || component.Hook.HandlerType == "command") {
			bindings = append(bindings, ir.SetupBinding{ComponentID: component.LogicalID, Target: ir.SetupBindingProcessEnv, Name: "CLAUDE_PLUGIN_OPTION_" + key})
		}
	}
	return bindings
}

func setupVariableDependencies(components []ir.Component, key string) []string {
	var out []string
	placeholderCandidates := []string{"${" + key + "}", "${user_config." + key + "}", "${env:" + key + "}"}
	for _, component := range components {
		if component.Variable != nil {
			continue
		}
		raw, _ := json.Marshal(component)
		text := string(raw)
		for _, placeholder := range placeholderCandidates {
			if strings.Contains(text, placeholder) {
				out = append(out, component.LogicalID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func finalizeSetupRequirements(plugin *ir.NormalizedPlugin) error {
	seen := map[string]bool{}
	componentIDs := map[string]bool{}
	componentByID := map[string]ir.Component{}
	for _, component := range plugin.Components {
		componentIDs[component.LogicalID] = true
		componentByID[component.LogicalID] = component
	}
	for index := range plugin.Setup {
		requirement := &plugin.Setup[index]
		if requirement.Kind == ir.SetupSecret && requirement.Field != nil {
			if err := validateSecretPlacement(plugin.Components, requirement.Field.Key); err != nil {
				return fmt.Errorf("setup requirement %q: %w", requirement.ID, err)
			}
		}
		if seen[requirement.ID] {
			return fmt.Errorf("duplicate plugin setup requirement id %q", requirement.ID)
		}
		seen[requirement.ID] = true
		for _, binding := range requirement.Bindings {
			if !componentIDs[binding.ComponentID] {
				return fmt.Errorf("setup requirement %q references unknown component %q", requirement.ID, binding.ComponentID)
			}
			if !containsString(requirement.ComponentIDs, binding.ComponentID) {
				requirement.ComponentIDs = append(requirement.ComponentIDs, binding.ComponentID)
			}
			if component := componentByID[binding.ComponentID]; component.MCP != nil {
				if requirement.MCPServerName != "" && requirement.MCPServerName != component.MCP.ShortName {
					return fmt.Errorf("setup requirement %q cannot bind multiple MCP servers", requirement.ID)
				}
				requirement.MCPServerName = component.MCP.ShortName
			}
		}
		sort.Strings(requirement.ComponentIDs)
		requirement.ComponentIDs = compactStrings(requirement.ComponentIDs)
		requirement.Fingerprint = setupFingerprint(*requirement)
	}
	sort.SliceStable(plugin.Setup, func(i, j int) bool { return plugin.Setup[i].ID < plugin.Setup[j].ID })
	return nil
}

func validateSecretPlacement(components []ir.Component, key string) error {
	markers := []string{"${" + key + "}", "${user_config." + key + "}", "${env:" + key + "}"}
	contains := func(values ...string) bool {
		for _, value := range values {
			for _, marker := range markers {
				if strings.Contains(value, marker) {
					return true
				}
			}
		}
		return false
	}
	for _, component := range components {
		if component.MCP != nil && contains(append([]string{component.MCP.Command, component.MCP.Cwd, component.MCP.URL, component.MCP.OAuthResource, component.MCP.OAuthClientID, component.MCP.OAuthClientMetadataURL}, component.MCP.Args...)...) {
			return fmt.Errorf("Secret %s may only bind to MCP env/header or OAuth transport", key)
		}
		if component.Hook != nil && contains(component.Hook.Command, component.Hook.URL, component.Hook.Prompt) {
			return fmt.Errorf("Secret %s may only bind to Hook env/header", key)
		}
		if component.Agent != nil && contains(component.Agent.Prompt, component.Agent.Description) {
			return fmt.Errorf("Secret %s must not enter Agent prompt", key)
		}
		if component.Command != nil && contains(component.Command.PromptTemplate) {
			return fmt.Errorf("Secret %s must not enter Command prompt", key)
		}
	}
	return nil
}

func setupFingerprint(requirement ir.SetupRequirement) string {
	requirement.Fingerprint = ""
	requirement.SourceDialect = ""
	requirement.SourcePath = ""
	requirement.Raw = nil
	raw, _ := json.Marshal(requirement)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	for _, value := range values {
		if value == "" || len(out) > 0 && out[len(out)-1] == value {
			continue
		}
		out = append(out, value)
	}
	return out
}
