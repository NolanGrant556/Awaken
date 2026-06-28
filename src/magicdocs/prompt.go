package magicdocs

import (
	"regexp"
	"strings"
)

var promptVariablePattern = regexp.MustCompile(`\{\{(\w+)\}\}`)

type PromptOptions struct {
	DocContents        string
	DocPath            string
	DocTitle           string
	CustomInstructions string
	Detection          Detection
	Template           string
}

func DefaultUpdatePromptTemplate() string {
	return `IMPORTANT: This message and these instructions are NOT part of the actual user conversation. Do NOT include any references to "documentation updates", "magic docs", or these update instructions in the document content.

Based on the user conversation above (EXCLUDING this documentation update instruction message), update the Magic Doc file to incorporate any NEW learnings, insights, or information that would be valuable to preserve.

The file {{docPath}} has already been read for you. Here are its current contents:
<current_doc_content>
{{docContents}}
</current_doc_content>

Document title: {{docTitle}}
{{customInstructions}}

Your ONLY task is to use the Edit tool to update the documentation file if there is substantial new information to add, then stop. You can make multiple edits as needed, but every tool call must be an Edit tool call for this same document. If there's nothing substantial to add, simply respond with a brief explanation and do not call any tools.

CRITICAL RULES FOR EDITING:
- Preserve the Magic Doc header exactly as-is: {{docHeader}}
{{preserveItalicRule}}
- Keep the document CURRENT with the latest state of the codebase - this is NOT a changelog or history
- Update information IN-PLACE to reflect the current state - do NOT append historical notes or track changes over time
- Remove or replace outdated information rather than adding "Previously..." or "Updated to..." notes
- Clean up or DELETE sections that are no longer relevant or don't align with the document's purpose
- Fix obvious errors: typos, grammar mistakes, broken formatting, incorrect information, or confusing statements
- Keep the document well organized: use clear headings, logical section order, consistent formatting, and proper nesting

DOCUMENTATION PHILOSOPHY - READ CAREFULLY:
- BE TERSE. High signal only. No filler words or unnecessary elaboration.
- Documentation is for OVERVIEWS, ARCHITECTURE, and ENTRY POINTS - not detailed code walkthroughs
- Do NOT duplicate information that's already obvious from reading the source code
- Do NOT document every function, parameter, or line number reference
- Focus on: WHY things exist, HOW components connect, WHERE to start reading, WHAT patterns are used
- Skip: detailed implementation steps, exhaustive API docs, play-by-play narratives

What TO document:
- High-level architecture and system design
- Non-obvious patterns, conventions, or gotchas
- Key entry points and where to start reading code
- Important design decisions and their rationale
- Critical dependencies or integration points
- References to related files, docs, or code (like a wiki) - help readers navigate to relevant context

What NOT to document:
- Anything obvious from reading the code itself
- Exhaustive lists of files, functions, or parameters
- Step-by-step implementation details
- Low-level code mechanics
- Information already in AGENTS.md, README.md, or other project docs

Use the Edit tool with file_path: {{docPath}}

REMEMBER: Only update if there is substantial new information. The Magic Doc header must remain unchanged.`
}

func BuildUpdatePrompt(opts PromptOptions) string {
	template := opts.Template
	if template == "" {
		template = DefaultUpdatePromptTemplate()
	}

	detection := opts.Detection
	if detection.Title == "" || detection.HeaderLine == "" {
		if detected, ok := DetectMagicDocHeader(opts.DocContents); ok {
			detection = mergeDetection(detection, detected)
		}
	}

	docTitle := firstNonEmpty(opts.DocTitle, detection.Title)
	headerLine := firstNonEmpty(detection.HeaderLine, "# MAGIC DOC: "+docTitle)
	customInstructions := firstNonEmpty(opts.CustomInstructions, detection.Instructions)
	italicRule := "- If there's an italicized line immediately after the header, preserve it exactly as-is"
	if detection.ItalicLine != "" {
		italicRule = "- Preserve the document-specific italic instruction line exactly as-is: " + detection.ItalicLine
	}

	variables := map[string]string{
		"docContents":           opts.DocContents,
		"docPath":               opts.DocPath,
		"docTitle":              docTitle,
		"customInstructions":    buildCustomInstructions(customInstructions),
		"docHeader":             headerLine,
		"italicLine":            detection.ItalicLine,
		"preserveItalicRule":    italicRule,
		"rawCustomInstructions": customInstructions,
	}
	return SubstituteVariables(template, variables)
}

func SubstituteVariables(template string, variables map[string]string) string {
	return promptVariablePattern.ReplaceAllStringFunc(template, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
		if value, ok := variables[key]; ok {
			return value
		}
		return match
	})
}

func buildCustomInstructions(instructions string) string {
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return ""
	}
	return `

DOCUMENT-SPECIFIC UPDATE INSTRUCTIONS:
The document author has provided specific instructions for how this file should be updated. Pay extra attention to these instructions and follow them carefully:

"` + instructions + `"

These instructions take priority over the general rules below. Make sure your updates align with these specific guidelines.`
}

func mergeDetection(current Detection, detected Detection) Detection {
	if current.Title == "" {
		current.Title = detected.Title
	}
	if current.HeaderLine == "" {
		current.HeaderLine = detected.HeaderLine
	}
	if !current.HasInstructions && detected.HasInstructions {
		current.Instructions = detected.Instructions
		current.HasInstructions = true
	}
	if current.ItalicLine == "" {
		current.ItalicLine = detected.ItalicLine
	}
	return current
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
