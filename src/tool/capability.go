package tool

import (
	"sort"
	"strings"
)

// Capability is the provider-neutral vocabulary used to describe what a tool
// can do. A capability identifies user-visible semantics; it does not identify
// the implementation selected to perform them.
type Capability string

const (
	CapabilityFileRead           Capability = "file.read"
	CapabilityFileWrite          Capability = "file.write"
	CapabilityFileEdit           Capability = "file.edit"
	CapabilityFileDelete         Capability = "file.delete"
	CapabilityShellExecute       Capability = "shell.execute"
	CapabilityWebSearch          Capability = "web.search"
	CapabilityBrowserInteractV2  Capability = "browser.interact"
	CapabilityPDFRead            Capability = "pdf.read"
	CapabilityPDFRender          Capability = "pdf.render"
	CapabilityDocumentCreate     Capability = "document.create"
	CapabilityPresentationCreate Capability = "presentation.create"
	CapabilityPresentationEdit   Capability = "presentation.edit"
	CapabilityPresentationRender Capability = "presentation.render"
	CapabilitySpreadsheetCreate  Capability = "spreadsheet.create"
	CapabilityImageGenerate      Capability = "image.generate"
	CapabilityMessageSend        Capability = "message.send"
	CapabilityToolDiagnose       Capability = "tool.diagnose"
	CapabilityDependencyInstall  Capability = "dependency.install"
	CapabilityGitRemoteRead      Capability = "git.remote_read"
	CapabilityGitClone           Capability = "git.clone"
)

// NormalizeCapabilities removes empty/duplicate values and returns a stable
// lexical order. Stable ordering is important because capability sets are part
// of registry and run-snapshot identities.
func NormalizeCapabilities(values []Capability) []Capability {
	seen := make(map[Capability]struct{}, len(values))
	result := make([]Capability, 0, len(values))
	for _, value := range values {
		value = Capability(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func HasCapability(values []Capability, want Capability) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// inferCapabilitiesFromDefinition is a compatibility bridge for existing
// tools. New providers should author ToolDefinition.Capabilities explicitly;
// inference must never be used as an authorization decision.
func inferCapabilitiesFromDefinition(def ToolDefinition) []Capability {
	if len(def.Capabilities) > 0 {
		return NormalizeCapabilities(def.Capabilities)
	}

	name := strings.ToLower(strings.TrimSpace(def.Name))
	var capabilities []Capability
	switch name {
	case "read", "glob", "grep", "file_read", "artifact_read", "artifact_inspect":
		capabilities = append(capabilities, CapabilityFileRead)
	case "write":
		capabilities = append(capabilities, CapabilityFileWrite)
	case "edit":
		capabilities = append(capabilities, CapabilityFileEdit)
	case "trash_file":
		capabilities = append(capabilities, CapabilityFileDelete)
	case "bash":
		capabilities = append(capabilities, CapabilityShellExecute)
	case "websearch":
		capabilities = append(capabilities, CapabilityWebSearch)
	case "dependency_install":
		capabilities = append(capabilities, CapabilityDependencyInstall)
	case "git_remote":
		capabilities = append(capabilities, CapabilityGitRemoteRead, CapabilityGitClone)
	case "pdf_extract":
		capabilities = append(capabilities, CapabilityPDFRead)
	case "pdf_render":
		capabilities = append(capabilities, CapabilityPDFRender)
	case "document_create":
		capabilities = append(capabilities, CapabilityDocumentCreate)
	case "presentation_create":
		capabilities = append(capabilities, CapabilityPresentationCreate)
	case "presentation_edit":
		capabilities = append(capabilities, CapabilityPresentationEdit)
	case "presentation_render":
		capabilities = append(capabilities, CapabilityPresentationRender)
	case "spreadsheet_create":
		capabilities = append(capabilities, CapabilitySpreadsheetCreate)
	}

	switch def.Annotations.CapabilityClass {
	case CapabilityArtifactRead:
		capabilities = append(capabilities, CapabilityFileRead)
	case CapabilityArtifactWrite:
		if len(capabilities) == 0 {
			capabilities = append(capabilities, CapabilityFileWrite)
		}
	case CapabilityBrowserRead, CapabilityBrowserInteract:
		capabilities = append(capabilities, CapabilityBrowserInteractV2)
	case CapabilityConnectorWrite:
		// Connector-write is intentionally not assumed to be message.send or
		// file.delete. Those semantics require an input-aware provider manifest.
	}
	return NormalizeCapabilities(capabilities)
}
