package verifierinit

type AreaKind string

const (
	AreaUnknown   AreaKind = "unknown"
	AreaWebApp    AreaKind = "web_app"
	AreaAPI       AreaKind = "api"
	AreaCLI       AreaKind = "cli"
	AreaSharedLib AreaKind = "shared_lib"
)

type VerifierType string

const (
	VerifierPlaywright VerifierType = "playwright"
	VerifierAPI        VerifierType = "api"
	VerifierCLI        VerifierType = "cli"
)

type ProjectFile struct {
	Path    string
	Content string
}

type Area struct {
	Name           string
	Path           string
	Kind           AreaKind
	Manifest       string
	PackageManager string
	Stack          []string
	DevCommand     string
	ReadySignal    string
	BaseURL        string
	EntryCommand   string
	HasPlaywright  bool
}

type ToolState struct {
	PlaywrightPackage        bool
	PlaywrightMCP            bool
	ChromeDevToolsMCP        bool
	ClaudeChromeExtensionMCP bool
	Tmux                     bool
	Asciinema                bool
	Curl                     bool
	HTTPie                   bool
}

type AuthRequirement struct {
	Required       bool
	Method         string
	LoginURL       string
	EnvVars        []string
	PostLoginCheck string
}

type VerifierPlan struct {
	Name           string
	Type           VerifierType
	Area           Area
	Description    string
	AllowedTools   []string
	BrowserOptions []string
	Auth           AuthRequirement
}
