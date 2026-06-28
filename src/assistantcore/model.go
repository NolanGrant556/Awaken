package assistantcore

type AssistantSource string

const (
	AssistantSourceBuiltin   AssistantSource = "builtin"
	AssistantSourceGenerated AssistantSource = "generated"
	AssistantSourceUser      AssistantSource = "user"
)

type DefaultMode string

const (
	DefaultModeAuto  DefaultMode = "auto"
	DefaultModeFixed DefaultMode = "fixed"
)

type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModeAuto              PermissionMode = "auto"
	PermissionModeBubble            PermissionMode = "bubble"
)

type Assistant struct {
	ID           string                `json:"id"`
	Source       AssistantSource       `json:"source"`
	Profile      AssistantProfile      `json:"profile"`
	State        AssistantState        `json:"state"`
	Binding      RuntimeBinding        `json:"binding"`
	Rules        AssistantRules        `json:"rules"`
	Defaults     AssistantDefaults     `json:"defaults"`
	Capabilities AssistantCapabilities `json:"capabilities"`
	Preferences  AssistantPreferences  `json:"preferences"`
	Presets      []Preset              `json:"presets,omitempty"`
	Metadata     map[string]string     `json:"metadata,omitempty"`
}

type AssistantProfile struct {
	Name            string            `json:"name"`
	NameI18N        map[string]string `json:"name_i18n,omitempty"`
	Description     string            `json:"description,omitempty"`
	DescriptionI18N map[string]string `json:"description_i18n,omitempty"`
	Avatar          string            `json:"avatar,omitempty"`
}

type AssistantState struct {
	Enabled   bool `json:"enabled"`
	SortOrder int  `json:"sort_order"`
}

type RuntimeBinding struct {
	AgentID   string `json:"agent_id"`
	RuntimeID string `json:"runtime_id"`
	Source    string `json:"source,omitempty"`
	Backend   string `json:"backend,omitempty"`
}

type AssistantRules struct {
	Content      string `json:"content,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
	StorageMode  string `json:"storage_mode,omitempty"`
}

type ScalarDefault struct {
	Mode  DefaultMode `json:"mode"`
	Value string      `json:"value,omitempty"`
}

type ListDefault struct {
	Mode  DefaultMode `json:"mode"`
	Value []string    `json:"value,omitempty"`
}

type AssistantDefaults struct {
	Model      ScalarDefault `json:"model"`
	Permission ScalarDefault `json:"permission"`
	Skills     ListDefault   `json:"skills"`
	MCPServers ListDefault   `json:"mcps"`
}

type AssistantCapabilities struct {
	DefaultSkillIDs         []string `json:"default_skill_ids,omitempty"`
	CustomSkillNames        []string `json:"custom_skill_names,omitempty"`
	DisabledBuiltinSkillIDs []string `json:"default_disabled_builtin_skill_ids,omitempty"`
	RequiredMCPServerIDs    []string `json:"required_mcp_server_ids,omitempty"`
}

type AssistantPreferences struct {
	LastModelID                 string   `json:"last_model_id,omitempty"`
	LastPermissionMode          string   `json:"last_permission_value,omitempty"`
	LastSkillIDs                []string `json:"last_skill_ids,omitempty"`
	LastDisabledBuiltinSkillIDs []string `json:"last_disabled_builtin_skill_ids,omitempty"`
	LastMCPServerIDs            []string `json:"last_mcp_ids,omitempty"`
}

type Preset struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Enabled     bool              `json:"enabled"`
	SortOrder   int               `json:"sort_order"`
	Binding     RuntimeBinding    `json:"binding"`
	Rules       AssistantRules    `json:"rules"`
	Defaults    AssistantDefaults `json:"defaults"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
	Builtin     bool   `json:"builtin,omitempty"`
	AutoInject  bool   `json:"auto_inject,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type MCPServer struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Scope       string `json:"scope,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type RunOverrides struct {
	Binding                 *RuntimeBinding
	Model                   *string
	PermissionMode          *PermissionMode
	SkillIDs                []string
	DisabledBuiltinSkillIDs []string
	MCPServerIDs            []string
	Rules                   *string
	SystemPrompt            *string
}

type RunConfig struct {
	AssistantID             string
	AssistantName           string
	Binding                 RuntimeBinding
	Model                   string
	PermissionMode          PermissionMode
	SkillIDs                []string
	DisabledBuiltinSkillIDs []string
	MCPServerIDs            []string
	Rules                   string
	SystemPrompt            string
}
