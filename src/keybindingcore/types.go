package keybindingcore

type Platform string

const (
	PlatformMacOS   Platform = "macos"
	PlatformWindows Platform = "windows"
	PlatformLinux   Platform = "linux"
	PlatformWSL     Platform = "wsl"
	PlatformUnknown Platform = "unknown"
)

type Keystroke struct {
	Key   string
	Ctrl  bool
	Alt   bool
	Shift bool
	Meta  bool
	Super bool
}

type Chord []Keystroke

type Binding struct {
	Chord   Chord
	Action  *string
	Context string
}

type Block struct {
	Context  string
	Bindings map[string]*string
}

type InputKey struct {
	Input string
	Key   string

	Ctrl  bool
	Alt   bool
	Shift bool
	Meta  bool
	Super bool
}

type ResolveType string

const (
	ResolveMatch          ResolveType = "match"
	ResolveNone           ResolveType = "none"
	ResolveUnbound        ResolveType = "unbound"
	ResolveChordStarted   ResolveType = "chord_started"
	ResolveChordCancelled ResolveType = "chord_cancelled"
)

type ResolveResult struct {
	Type    ResolveType
	Action  string
	Pending Chord
}

type IssueType string

const (
	IssueParseError     IssueType = "parse_error"
	IssueReserved       IssueType = "reserved"
	IssueInvalidContext IssueType = "invalid_context"
	IssueInvalidAction  IssueType = "invalid_action"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type ValidationIssue struct {
	Type       IssueType
	Severity   Severity
	Message    string
	Key        string
	Context    string
	Action     string
	Suggestion string
}

func Action(action string) *string {
	return &action
}
