package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"awaken/src/tool"
)

const (
	ErrorCodeArtifactOutputInvalid         = "artifact_output_invalid"
	ErrorCodeArtifactOutputOversize        = "artifact_output_oversize"
	ErrorCodeArtifactOutputHashMismatch    = "artifact_output_hash_mismatch"
	ErrorCodeArtifactOutputPathDenied      = "artifact_output_path_denied"
	ErrorCodeArtifactOutputContentMismatch = "artifact_output_content_mismatch"
	ErrorCodeArtifactOutputCommitFailed    = "artifact_output_commit_failed"
)

// validateArtifactOutputContract validates only the optional host-publication
// bridge. Failure disables typed Artifact publication for this tool; it never
// prevents the underlying MCP call from reaching a connected server.
func (a *McpToolAdapter) validateArtifactOutputContract(capabilities []tool.Capability) string {
	contract := a.providerMetadata.ArtifactOutput
	if contract == nil {
		return ""
	}
	if !declaresArtifactOutputCapability(capabilities) {
		return "MCP artifact output requires an explicit artifact-writing capability"
	}
	if contract.MaxBytes < 0 {
		return "MCP artifact output max_bytes cannot be negative"
	}
	if contract.MaxBytes > maxMCPArtifactOutputBytes {
		return fmt.Sprintf("MCP artifact output max_bytes cannot exceed the host limit of %d bytes", maxMCPArtifactOutputBytes)
	}
	fields := []struct {
		name string
		path string
	}{
		{name: "bytes_base64_field", path: contract.BytesBase64Field},
		{name: "sha256_field", path: contract.SHA256Field},
		{name: "relative_path_field", path: contract.RelativePathField},
		{name: "content_type_field", path: contract.ContentTypeField},
	}
	if strings.TrimSpace(contract.TitleField) != "" {
		fields = append(fields, struct {
			name string
			path string
		}{name: "title_field", path: contract.TitleField})
	}
	seenPaths := make(map[string]string, len(fields))
	for _, field := range fields {
		field.path = strings.TrimSpace(field.path)
		if field.path == "" {
			return "MCP artifact output " + field.name + " is required"
		}
		if !validArtifactFieldPath(field.path) {
			return fmt.Sprintf("MCP artifact output %s %q is not a valid dotted field path", field.name, field.path)
		}
		if previous, exists := seenPaths[field.path]; exists {
			return fmt.Sprintf("MCP artifact output %s and %s cannot use the same field %q", previous, field.name, field.path)
		}
		seenPaths[field.path] = field.name
		if !schemaDeclaresRequiredStringPath(a.outputSchemaJSON, field.path) {
			return fmt.Sprintf("MCP artifact output %s %q must be a required string in the provider output schema", field.name, field.path)
		}
	}
	return ""
}

func validArtifactFieldPath(value string) bool {
	for _, segment := range strings.Split(value, ".") {
		if strings.TrimSpace(segment) == "" || strings.TrimSpace(segment) != segment {
			return false
		}
	}
	return true
}

func declaresArtifactOutputCapability(capabilities []tool.Capability) bool {
	for _, capability := range capabilities {
		switch capability {
		case tool.CapabilityFileWrite,
			tool.CapabilityFileEdit,
			tool.CapabilityPDFRender,
			tool.CapabilityDocumentCreate,
			tool.CapabilityPresentationCreate,
			tool.CapabilityPresentationEdit,
			tool.CapabilityPresentationRender,
			tool.CapabilitySpreadsheetCreate:
			return true
		}
	}
	return false
}

func schemaDeclaresRequiredStringPath(raw json.RawMessage, path string) bool {
	if len(raw) == 0 || strings.TrimSpace(path) == "" {
		return false
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return false
	}
	segments := strings.Split(path, ".")
	current := schema
	for index, segment := range segments {
		properties, ok := current["properties"].(map[string]any)
		if !ok || !schemaRequiredContains(current, segment) {
			return false
		}
		next, exists := properties[segment]
		if !exists {
			return false
		}
		nextSchema, ok := next.(map[string]any)
		if !ok {
			return false
		}
		if index == len(segments)-1 {
			return nextSchema["type"] == "string"
		}
		if value, exists := nextSchema["type"]; exists && value != "object" {
			return false
		}
		current = nextSchema
	}
	return false
}

func schemaRequiredContains(schema map[string]any, field string) bool {
	values, ok := schema["required"].([]any)
	if !ok {
		return false
	}
	for _, value := range values {
		if text, ok := value.(string); ok && text == field {
			return true
		}
	}
	return false
}

func valueAtPath(root any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	current := root
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := object[segment]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}
