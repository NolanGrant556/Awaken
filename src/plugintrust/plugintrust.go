package plugintrust

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type Manifest struct {
	Name        string   `json:"name,omitempty"`
	Version     string   `json:"version,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Description string   `json:"description,omitempty"`
	Commands    []string `json:"commands,omitempty"`
	Agents      []string `json:"agents,omitempty"`
	Skills      []string `json:"skills,omitempty"`
	MCPServers  []string `json:"mcpServers,omitempty"`
	Source      Source   `json:"-"`
}

type Source struct {
	Name        string `json:"name,omitempty"`
	Origin      string `json:"origin,omitempty"`
	URL         string `json:"url,omitempty"`
	Signature   string `json:"signature,omitempty"`
	Allowlisted bool   `json:"allowlisted,omitempty"`
}

type TrustStatus string

const (
	TrustTrusted              TrustStatus = "trusted"
	TrustUntrusted            TrustStatus = "untrusted"
	TrustUnknown              TrustStatus = "unknown"
	TrustRequiresConfirmation TrustStatus = "requires_confirmation"
)

type Finding struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type Report struct {
	Manifest Manifest    `json:"manifest,omitempty"`
	Errors   []Finding   `json:"errors,omitempty"`
	Warnings []Finding   `json:"warnings,omitempty"`
	Trust    TrustStatus `json:"trust,omitempty"`
	Message  string      `json:"message,omitempty"`
}

func (r Report) OK() bool {
	return len(r.Errors) == 0
}

func ValidateManifest(raw []byte) Report {
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return Report{
			Errors: []Finding{{
				Path:    "json",
				Message: fmt.Sprintf("invalid JSON manifest: %v", err),
			}},
		}
	}
	return ValidateManifestMap(values)
}

func ValidateManifestMap(values map[string]any) Report {
	report := Report{}
	if values == nil {
		report.Errors = append(report.Errors, Finding{
			Path:    "manifest",
			Message: "manifest must be a JSON object",
		})
		return report
	}

	for _, key := range sortedKeys(values) {
		if marketplaceOnlyManifestFields[key] {
			report.Warnings = append(report.Warnings, Finding{
				Path:    key,
				Message: fmt.Sprintf("field %q belongs in marketplace metadata, not plugin manifest", key),
			})
			continue
		}
		if !knownManifestFields[key] {
			report.Warnings = append(report.Warnings, Finding{
				Path:    key,
				Message: fmt.Sprintf("unknown manifest field %q will be ignored", key),
			})
		}
	}

	report.Manifest.Name = readString(values, "name", &report)
	report.Manifest.Version = readString(values, "version", &report)
	report.Manifest.Homepage = readString(values, "homepage", &report)
	report.Manifest.Description = readString(values, "description", &report)
	report.Manifest.Commands = readStringList(values, "commands", &report)
	report.Manifest.Agents = readStringList(values, "agents", &report)
	report.Manifest.Skills = readStringList(values, "skills", &report)
	report.Manifest.MCPServers = readStringList(values, "mcpServers", &report)

	if strings.TrimSpace(report.Manifest.Name) == "" {
		report.Errors = append(report.Errors, Finding{
			Path:    "name",
			Message: "plugin name is required",
		})
	} else if containsSpace(report.Manifest.Name) {
		report.Errors = append(report.Errors, Finding{
			Path:    "name",
			Message: "plugin name must not contain spaces",
		})
	} else if !kebabCasePattern.MatchString(report.Manifest.Name) {
		report.Warnings = append(report.Warnings, Finding{
			Path:    "name",
			Message: fmt.Sprintf("plugin name %q is not kebab-case", report.Manifest.Name),
		})
	}

	checkPathTraversal("commands", report.Manifest.Commands, &report)
	checkPathTraversal("agents", report.Manifest.Agents, &report)
	checkPathTraversal("skills", report.Manifest.Skills, &report)
	checkPathTraversal("mcpServers", report.Manifest.MCPServers, &report)

	return report
}

func AssessTrust(source Source) Report {
	origin := strings.ToLower(strings.TrimSpace(source.Origin))
	sourceName := displaySource(source)
	report := Report{Trust: TrustUnknown}

	if source.Allowlisted || origin == "official" {
		report.Trust = TrustTrusted
		report.Message = fmt.Sprintf("Plugin source %s is trusted.", sourceName)
		return report
	}

	switch origin {
	case "blocked", "denylisted", "untrusted":
		report.Trust = TrustUntrusted
		report.Message = fmt.Sprintf("Plugin source %s is not trusted. Do not install, update, or use this plugin until the source is reviewed.", sourceName)
	case "remote", "git", "github", "http", "https":
		report.Trust = TrustRequiresConfirmation
		report.Message = trustWarningMessage(sourceName)
		report.Warnings = append(report.Warnings, Finding{
			Path:    "source",
			Message: report.Message,
		})
	default:
		if isRemoteURL(source.URL) {
			report.Trust = TrustRequiresConfirmation
			report.Message = trustWarningMessage(sourceName)
			report.Warnings = append(report.Warnings, Finding{
				Path:    "source",
				Message: report.Message,
			})
			return report
		}
		if source.Signature != "" {
			report.Message = fmt.Sprintf("Plugin source %s has a signature, but no trusted origin or allowlist entry was provided.", sourceName)
			report.Warnings = append(report.Warnings, Finding{
				Path:    "signature",
				Message: report.Message,
			})
			return report
		}
		report.Message = fmt.Sprintf("Plugin source %s has unknown trust. Review it before installing, updating, or using it.", sourceName)
	}

	return report
}

var kebabCasePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

var knownManifestFields = map[string]bool{
	"name":        true,
	"version":     true,
	"homepage":    true,
	"description": true,
	"commands":    true,
	"agents":      true,
	"skills":      true,
	"mcpServers":  true,
}

var marketplaceOnlyManifestFields = map[string]bool{
	"category": true,
	"source":   true,
	"tags":     true,
	"strict":   true,
	"id":       true,
}

func readString(values map[string]any, field string, report *Report) string {
	value, ok := values[field]
	if !ok || value == nil {
		return ""
	}
	s, ok := value.(string)
	if !ok {
		report.Errors = append(report.Errors, Finding{
			Path:    field,
			Message: fmt.Sprintf("%s must be a string", field),
		})
		return ""
	}
	return s
}

func readStringList(values map[string]any, field string, report *Report) []string {
	value, ok := values[field]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return append([]string(nil), typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for i, item := range typed {
			s, ok := item.(string)
			if !ok {
				report.Errors = append(report.Errors, Finding{
					Path:    fmt.Sprintf("%s[%d]", field, i),
					Message: fmt.Sprintf("%s entries must be strings", field),
				})
				continue
			}
			items = append(items, s)
		}
		return items
	default:
		report.Errors = append(report.Errors, Finding{
			Path:    field,
			Message: fmt.Sprintf("%s must be a string list", field),
		})
		return nil
	}
}

func checkPathTraversal(field string, paths []string, report *Report) {
	for i, path := range paths {
		if strings.Contains(path, "..") {
			report.Errors = append(report.Errors, Finding{
				Path:    fmt.Sprintf("%s[%d]", field, i),
				Message: fmt.Sprintf("%s path contains \"..\": %s", field, path),
			})
		}
	}
}

func containsSpace(value string) bool {
	return strings.ContainsFunc(value, unicode.IsSpace)
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isRemoteURL(url string) bool {
	normalized := strings.ToLower(strings.TrimSpace(url))
	return strings.HasPrefix(normalized, "http://") || strings.HasPrefix(normalized, "https://") || strings.HasPrefix(normalized, "git://")
}

func displaySource(source Source) string {
	if source.Name != "" {
		return fmt.Sprintf("%q", source.Name)
	}
	if source.URL != "" {
		return fmt.Sprintf("%q", source.URL)
	}
	if source.Origin != "" {
		return fmt.Sprintf("%q", source.Origin)
	}
	return "\"unknown\""
}

func trustWarningMessage(sourceName string) string {
	return fmt.Sprintf("Confirm you trust plugin source %s before installing, updating, or using it. Plugins can include MCP servers, files, or other software that Awaken cannot verify automatically.", sourceName)
}
