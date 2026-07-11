package artifactruntime

import (
	"strings"

	"awaken/src/tool"
)

func completeTypedArtifactInputSchema(schema tool.ToolParametersSchema) tool.ToolParametersSchema {
	completed := schema
	completed.Required = append([]string(nil), schema.Required...)
	completed.Properties = make(map[string]tool.ToolParam, len(schema.Properties))
	for name, parameter := range schema.Properties {
		completed.Properties[name] = completeTypedArtifactSchemaNode(name, parameter)
	}
	completed.OneOf = make([]tool.ToolParam, len(schema.OneOf))
	for index, option := range schema.OneOf {
		completed.OneOf[index] = completeTypedArtifactSchemaNode("option", option)
	}
	return completed
}

func completeTypedArtifactSchemaNode(name string, node tool.ToolParam) tool.ToolParam {
	completed := node
	completed.Required = append([]string(nil), node.Required...)
	completed.Enum = append([]any(nil), node.Enum...)
	if strings.TrimSpace(completed.Description) == "" {
		completed.Description = typedArtifactFieldDescription(name)
	}
	completed.Properties = make(map[string]tool.ToolParam, len(node.Properties))
	for childName, child := range node.Properties {
		completed.Properties[childName] = completeTypedArtifactSchemaNode(childName, child)
	}
	if completed.Type == "object" && completed.AdditionalProperties == nil {
		completed.AdditionalProperties = tool.BoolPtr(false)
	}
	if node.Items != nil {
		item := completeTypedArtifactSchemaNode(strings.TrimSuffix(name, "s")+" item", *node.Items)
		completed.Items = &item
	} else if completed.Type == "array" {
		var item tool.ToolParam
		switch name {
		case "rows":
			cell := tool.ToolParam{Type: "string", Description: "Cell text value."}
			item = tool.ToolParam{Type: "array", Description: "One spreadsheet row.", Items: &cell}
		case "paragraphs":
			item = tool.ToolParam{Type: "string", Description: "One document paragraph."}
		}
		if item.Type != "" {
			completed.Items = &item
		}
	}
	completed.OneOf = make([]tool.ToolParam, len(node.OneOf))
	for index, option := range node.OneOf {
		completed.OneOf[index] = completeTypedArtifactSchemaNode(name+" option", option)
	}
	return completed
}

func typedArtifactFieldDescription(name string) string {
	if description, ok := map[string]string{
		"accent": "Accent color value.", "background": "Background color value.",
		"body": "Primary body text.", "bullets": "Bullet text values.",
		"columns": "Structured column content.", "images": "Images placed on the slide.",
		"layout": "Supported slide layout.", "metrics": "Metric cards rendered on the slide.",
		"notes": "Speaker notes.", "subtitle": "Slide subtitle.", "timeline": "Timeline steps.",
		"title": "Display title.", "font_ea": "East Asian font family.",
		"font_latin": "Latin font family.", "muted": "Muted theme color.",
		"primary": "Primary theme color.", "surface": "Surface theme color.",
		"text": "Text theme color.", "label": "Metric or step label.",
		"value": "Metric display value.", "detail": "Short supporting detail.",
		"alt": "Accessible image alternative text.", "caption": "Image caption.",
		"fit": "Image fitting mode.", "path": "Task-scoped input path.",
		"x": "Horizontal position in slide units.", "y": "Vertical position in slide units.",
		"width": "Width in slide units.", "height": "Height in slide units.",
		"op": "Incremental edit operation.", "slide": "Complete slide payload.",
		"slide_index": "Zero-based slide index.", "theme": "Presentation theme overrides.",
		"edits": "Ordered incremental slide edits.", "output_path": "Writable artifact output path.",
		"output_dir": "Writable preview output directory.", "source_path": "Existing source artifact path.",
		"render_preview": "Whether to retain render preview files after verification.",
		"rows":           "Spreadsheet row values.", "paragraphs": "Document paragraphs.",
	}[name]; ok {
		return description
	}
	label := strings.ReplaceAll(strings.TrimSpace(name), "_", " ")
	if label == "" {
		label = "value"
	}
	return "Structured " + label + "."
}

// typedArtifactDefinition gives the artifact catalog one authoring path. The
// executor-specific schema still comes from typedArtifactTool.InputSchema;
// this file owns only shared selection, effect, permission and failure facts.
func typedArtifactDefinition(t typedArtifactTool) tool.ToolDefinition {
	annotations := tool.ToolAnnotations{
		ReadOnly:        t.readOnly,
		Destructive:     false,
		OpenWorld:       false,
		CapabilityClass: t.capability,
		TrustedOrigin:   tool.TrustedOriginBuiltin,
		FileBridgeScope: fileBridgeScopeForTool(t.readOnly),
		NetworkScope:    tool.NetworkScopeNone,
	}
	purpose := strings.TrimSpace(t.description)
	whenToUse := []string{"Use when the requested artifact operation and file type directly match " + t.name + "."}
	whenNotToUse := []string{
		"Plain UTF-8 source, Markdown, or JSON reading belongs to read_file; exact text replacement belongs to text_edit.",
		"Do not substitute this tool for a different document, spreadsheet, PDF, image, or presentation operation.",
	}
	effects := []tool.ToolSideEffect{{
		Kind: tool.SideEffectRead, Resources: []string{"task-scoped artifact inputs"},
		Description: "Reads artifact or runtime metadata without modifying the input.", Reversibility: tool.ReversibilityNotApplicable,
	}}
	if !t.readOnly {
		kind := typedArtifactMutationKind(t.name)
		description := "Creates typed artifact outputs inside the task writable/output scope."
		if kind == tool.SideEffectUpdate {
			description = "Updates an existing typed artifact (or writes the explicitly requested edit destination) inside the task writable/output scope."
		}
		effects = []tool.ToolSideEffect{{
			Kind: kind, Resources: []string{"declared artifact output paths"},
			Description: description, Reversibility: tool.ReversibilityConditional,
		}}
	}
	failureModes := []tool.ToolFailureMode{
		{Code: tool.ErrorCodeValidationError, Cause: "Artifact parameters, type, or output path are invalid.", Retryable: false, Recovery: []string{"Correct the typed parameters and use the matching artifact tool."}},
	}
	if t.requiresRuntime {
		failureModes = append(failureModes, tool.ToolFailureMode{
			Code: tool.ErrorCodeNeedsRuntime, Cause: "A required local artifact runtime is unavailable.", Retryable: false,
			Recovery: []string{"Prepare the reported runtime before issuing a new attempt."},
		})
	}
	failureModes = append(failureModes, tool.ToolFailureMode{
		Code: tool.ErrorCodeToolExecutionFailed, Cause: "The artifact engine failed during execution or verification.", Retryable: false,
		Recovery: []string{"Inspect diagnostics and verify input/output state before issuing a new attempt."},
	})
	return tool.BuildBuiltinDefinition(t, tool.ToolDefinition{
		Name: t.name, DisplayName: t.displayName, Description: t.description,
		Purpose: purpose, WhenToUse: whenToUse, WhenNotToUse: whenNotToUse,
		OutputSchema: typedArtifactDetailsSchema(), SideEffects: effects,
		Examples: typedArtifactExamples(t.name), FailureModes: failureModes,
		Annotations: annotations,
	})
}

func typedArtifactExamples(name string) []tool.ToolExample {
	var scenario string
	var input map[string]any
	switch strings.TrimSpace(name) {
	case "workspace_dependencies":
		scenario, input = "Inspect the prepared workspace runtime dependency manifest.", map[string]any{}
	case "runtime_preflight":
		scenario, input = "Check local artifact runtimes before authoring or rendering.", map[string]any{}
	case "artifact_read", "artifact_inspect":
		scenario, input = "Inspect a task-scoped PDF artifact.", map[string]any{"path": "report.pdf"}
	case "artifact_render":
		scenario, input = "Render a presentation to a preview directory.", map[string]any{"path": "deck.pptx", "output_dir": "deck_preview"}
	case "artifact_export":
		scenario, input = "Export an existing artifact into the output scope.", map[string]any{"path": "source.pdf", "output_path": "output/source.pdf"}
	case "spreadsheet_create", "spreadsheet_edit":
		scenario, input = "Write a small table to an Excel workbook.", map[string]any{
			"output_path": "table.xlsx", "rows": []any{[]any{"Name", "Value"}, []any{"A", "1"}},
		}
	case "document_create", "document_edit":
		scenario, input = "Write a short Word report.", map[string]any{
			"output_path": "report.docx", "title": "Report", "paragraphs": []any{"Summary"},
		}
	case "office_export_pdf":
		scenario, input = "Convert a completed Word report to a real PDF.", map[string]any{
			"path": "report.docx", "output_path": "report.pdf",
		}
	case "presentation_create":
		scenario, input = "Create a one-slide presentation and retain render verification.", map[string]any{
			"output_path": "deck.pptx",
			"slides":      []any{map[string]any{"title": "Summary", "layout": "bullets", "bullets": []any{"Result"}}},
		}
	case "presentation_edit":
		scenario, input = "Update the title of the first slide without rewriting other slides.", map[string]any{
			"source_path": "deck.pptx",
			"edits":       []any{map[string]any{"op": "update", "slide_index": 0, "title": "Updated Summary"}},
		}
	case "presentation_render":
		scenario, input = "Render an existing presentation for layout verification.", map[string]any{"path": "deck.pptx", "output_dir": "deck_preview"}
	case "pdf_extract":
		scenario, input = "Extract text and page facts from a PDF.", map[string]any{"path": "report.pdf"}
	case "image_inspect":
		scenario, input = "Inspect image dimensions and format.", map[string]any{"path": "chart.png"}
	case "image_convert":
		scenario, input = "Convert a PNG image to JPEG.", map[string]any{"path": "chart.png", "output_path": "output/chart.jpg"}
	case "media_probe":
		scenario, input = "Inspect basic metadata for a local video.", map[string]any{"path": "clip.mp4"}
	default:
		return nil
	}
	return []tool.ToolExample{{Scenario: scenario, Input: input, Outcome: "Returns canonical status plus schema-backed artifact details and artifacts when produced."}}
}

func typedArtifactMutationKind(name string) string {
	if strings.HasSuffix(strings.TrimSpace(name), "_edit") {
		return tool.SideEffectUpdate
	}
	return tool.SideEffectCreate
}

func typedArtifactAction(name string) tool.ArtifactAction {
	if typedArtifactMutationKind(name) == tool.SideEffectUpdate {
		return tool.ArtifactActionUpdated
	}
	return tool.ArtifactActionCreated
}

func typedArtifactDetailsSchema() tool.ToolParam {
	return tool.ToolParam{Type: "object", AdditionalProperties: tool.BoolPtr(false), Properties: map[string]tool.ToolParam{
		"path":            {Type: "string", Description: "Primary artifact path when produced or inspected."},
		"artifact_status": {Type: "string", Description: "Tool-specific artifact readiness or verification status."},
		"pdf_path":        {Type: "string", Description: "Rendered PDF path when the operation produces one."},
		"preview_images":  {Type: "array", Description: "Rendered preview image paths.", Items: &tool.ToolParam{Type: "string"}},
		"runtime_missing": {Type: "array", Description: "Missing runtime capability names.", Items: &tool.ToolParam{Type: "string"}},
		"result":          {Type: "object", Description: "Structured tool-specific result returned by the artifact executor.", AdditionalProperties: tool.BoolPtr(true)},
		"result_text":     {Type: "string", Description: "Bounded textual result when the executor does not return a JSON object."},
	}}
}
