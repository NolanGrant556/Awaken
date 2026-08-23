// Package plugins owns Agent Plugins 1.0.0 package loading, install
// lifecycle, and the source roots / MCP configs handed to the existing
// Skill and MCP runtimes.
package plugins

import (
	"errors"
	"time"

	"awaken/src/mcpclient"
)

const (
	SchemaPluginV1 = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	SchemaMCPV1    = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

	SchemaPrefix = "https://agent-plugins.org/schemas/"

	PluginIdentityPrefix = "plugin__"
	StateVersion         = 1

	MaxDownloadBytes = 100 << 20
	MaxExtractBytes  = 500 << 20
	MaxPackageFiles  = 10000
)

var (
	ErrUnsupportedSchema      = errors.New("plugin_unsupported_schema")
	ErrManifestInvalid        = errors.New("plugin_manifest_invalid")
	ErrMCPInvalid             = errors.New("plugin_mcp_invalid")
	ErrAlreadyExists          = errors.New("plugin already exists")
	ErrReplaceTrashUnavailable = errors.New("plugin replacement Trash unavailable")
	ErrNotFound               = errors.New("plugin not found")
	ErrStateCorrupt           = errors.New("plugins.json is corrupt")
	ErrHomeRequired           = errors.New("AWAKEN_HOME is required")
	ErrPathEscapes            = errors.New("path escapes the plugin root")
)

// Diagnostic is a component-level or non-fatal loader finding.
type Diagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// Author is the optional plugin.json author object.
type Author struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Manifest is the portable Agent Plugins 1.0.0 identity document.
type Manifest struct {
	Schema      string
	Name        string
	Version     string
	Description string
	Author      *Author
	Homepage    string
	Repository  string
	License     string
	Keywords    []string
}

// SourceKind identifies how a plugin was installed.
type SourceKind string

const (
	SourceLocalPath SourceKind = "local_path"
	SourceRemoteURL SourceKind = "remote_url"
	SourceRemoteGit SourceKind = "remote_git"
)

// Source is the durable install origin recorded in plugins.json.
type Source struct {
	Kind    SourceKind `json:"kind"`
	Path    string     `json:"path,omitempty"`
	URL     string     `json:"url,omitempty"`
	Ref     string     `json:"ref,omitempty"`
	Subpath string     `json:"subpath,omitempty"`
}

// MCPServerToggle is the per-server enable override stored in plugins.json.
type MCPServerToggle struct {
	Enabled bool `json:"enabled"`
}

// Record is one plugins.json entry.
type Record struct {
	Enabled     bool                       `json:"enabled"`
	Version     string                     `json:"version,omitempty"`
	Source      Source                     `json:"source"`
	InstalledAt time.Time                  `json:"installedAt"`
	Digest      string                     `json:"digest"`
	MCPServers  map[string]MCPServerToggle `json:"mcpServers,omitempty"`
}

// StateFile is the on-disk plugins.json shape.
type StateFile struct {
	Version int               `json:"version"`
	Plugins map[string]Record `json:"plugins"`
}

// LoadedServer is one translated plugin MCP server.
type LoadedServer struct {
	ShortName string
	Identity  string
	Config    mcpclient.McpServerConfig
	Enabled   bool
	Invalid   *Diagnostic
}

// InstalledPlugin is the runtime view of one installed package.
type InstalledPlugin struct {
	Name        string
	Root        string
	DataDir     string
	Manifest    Manifest
	Enabled     bool
	Source      Source
	Digest      string
	InstalledAt time.Time
	SkillRoot   string
	MCPServers  []LoadedServer
	Diagnostics []Diagnostic
}

// InstallRequest is the user-facing install input.
type InstallRequest struct {
	Source  Source
	Replace bool
}

// InstallResult reports a successful install or replace.
type InstallResult struct {
	Plugin               *InstalledPlugin
	Replaced             bool
	PreviousTrashedPath  string
	Message              string
}

func warning(code, message string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "warning"}
}

func errorDiag(code, message string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error"}
}
