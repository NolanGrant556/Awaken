package memory

import "time"

type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

type Type string

const (
	TypeUser      Type = "user"
	TypeFeedback  Type = "feedback"
	TypeProject   Type = "project"
	TypeReference Type = "reference"
)

type Status string

const (
	StatusActive     Status = "active"
	StatusSuperseded Status = "superseded"
)

// Record is the parsed authority held by one Markdown topic file. Unknown
// frontmatter fields are retained across updates for forward compatibility.
type Record struct {
	SchemaVersion  int            `yaml:"schema_version" json:"schema_version"`
	MemoryID       string         `yaml:"memory_id" json:"memory_id"`
	Name           string         `yaml:"name" json:"name"`
	Description    string         `yaml:"description" json:"description"`
	Type           Type           `yaml:"type" json:"type"`
	Status         Status         `yaml:"status" json:"status"`
	Revision       int            `yaml:"revision" json:"revision"`
	UpdatedAt      time.Time      `yaml:"updated_at" json:"updated_at"`
	SourceThreadID string         `yaml:"source_thread_id" json:"source_thread_id"`
	SourceRecordID string         `yaml:"source_record_id" json:"source_record_id"`
	CreatedAt      *time.Time     `yaml:"created_at,omitempty" json:"created_at,omitempty"`
	ObservedAt     *time.Time     `yaml:"observed_at,omitempty" json:"observed_at,omitempty"`
	ExpiresAt      *time.Time     `yaml:"expires_at,omitempty" json:"expires_at,omitempty"`
	CapturedBy     string         `yaml:"captured_by,omitempty" json:"captured_by,omitempty"`
	Supersedes     []string       `yaml:"supersedes,omitempty" json:"supersedes,omitempty"`
	Keywords       []string       `yaml:"keywords,omitempty" json:"keywords,omitempty"`
	Extra          map[string]any `yaml:",inline" json:"-"`

	Scope   Scope  `yaml:"-" json:"scope"`
	Path    string `yaml:"-" json:"-"`
	Content string `yaml:"-" json:"content,omitempty"`
	Action  string `yaml:"-" json:"action,omitempty"`
}

type Source struct {
	ThreadID   string
	RecordID   string
	CapturedBy string
}

type CreateInput struct {
	Scope       Scope
	Type        Type
	Name        string
	Description string
	Content     string
	Keywords    []string
	Source      Source
}

type Patch struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Content     *string  `json:"content,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

type Candidate struct {
	Filename    string    `json:"filename"`
	MemoryID    string    `json:"memory_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        Type      `json:"type"`
	Scope       Scope     `json:"scope"`
	Revision    int       `json:"revision"`
	UpdatedAt   time.Time `json:"updated_at"`
	ModTime     time.Time `json:"mtime"`
}
