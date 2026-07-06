package agentcore

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	shelltool "awaken/src/tools/shell"
)

// transformCtxKey is the context key used to track transform depth, enforcing
// the one-level transform guard: a transformed tool call must not itself
// produce another transform decision.
type transformCtxKey struct{}

// transformDepthFromCtx returns the transform depth stored in ctx. Zero means
// the call is not the result of a transform.
func transformDepthFromCtx(ctx stdcontext.Context) int {
	if v, ok := ctx.Value(transformCtxKey{}).(int); ok {
		return v
	}
	return 0
}

// ctxWithTransformDepth returns a new context with the given transform depth.
func ctxWithTransformDepth(ctx stdcontext.Context, depth int) stdcontext.Context {
	return stdcontext.WithValue(ctx, transformCtxKey{}, depth)
}

// TransformShellToFileChain attempts to rewrite a shell command into a safer
// apply_patch call. It only handles the most obvious cases:
//   - echo "..." > path   (write/overwrite)
//   - echo "..." >> path  (append, or create if the file does not exist)
//
// For anything else (sed -i, tee, pipelines, open-world commands), it returns
// ok=false with a reason explaining why the command cannot be transformed.
//
// workspaceRoot is the directory the resulting apply_patch path should be
// relative to. If the redirect target resolves outside workspaceRoot, the
// transform is rejected.
func TransformShellToFileChain(command string, cwd string, workspaceRoot string) (newToolName string, newInput map[string]any, ok bool, reason string) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", nil, false, "empty shell command"
	}

	// Reject compound commands outright; only simple `echo > path` is transformable.
	if hasCompoundOperator(command) {
		return "", nil, false, "compound shell commands cannot be transformed; use apply_patch directly"
	}

	// Parse `echo <content> (>|>>) <path>` first. If the command is not an
	// echo redirect, fall back to a tier-based rejection below so that
	// open-world commands like `curl` still surface an open-world reason.
	content, path, appendMode, parsed := parseEchoRedirect(command)
	if !parsed {
		tier := shelltool.ClassifyShellCommand(command, cwd)
		if tier == shelltool.ShellTierOpenWorldSystem {
			return "", nil, false, "open-world shell command cannot be transformed; use apply_patch directly"
		}
		return "", nil, false, "shell command is not a simple echo redirect; use apply_patch directly"
	}

	// Resolve the target path against cwd.
	resolved := resolveTransformPath(path, cwd)

	// Only transform if the path is within the workspace root. This catches
	// echo redirects to absolute paths outside the workspace (e.g. /etc/passwd)
	// with a more specific reason than the open-world fallback above.
	if !isPathWithinRoot(resolved, workspaceRoot) {
		return "", nil, false, "redirect target is outside the workspace; use apply_patch with a workspace path"
	}

	// Compute the workspace-relative path for the patch.
	rel, err := filepath.Rel(workspaceRoot, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", nil, false, "redirect target is outside the workspace; use apply_patch with a workspace path"
	}
	rel = filepath.ToSlash(rel)

	// Determine the new file content. echo always appends a trailing newline.
	newContent := content + "\n"
	if appendMode {
		existing, err := os.ReadFile(resolved)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", nil, false, "cannot read existing file for append transform: " + err.Error()
			}
			// File does not exist; treat as create.
		} else {
			existingStr := string(existing)
			if existingStr != "" && !strings.HasSuffix(existingStr, "\n") {
				existingStr += "\n"
			}
			newContent = existingStr + content + "\n"
		}
	}

	patch := buildAddFilePatch(rel, newContent)
	return "apply_patch", map[string]any{"patch": patch}, true, "echo redirect rewritten to apply_patch Add File"
}

// decideShellTransform applies the shell→apply_patch transform policy to a
// shell tool decision request. It returns (decision, true) if the request was
// handled (either transformed or denied with a reason), or (zero, false) if
// the request should fall through to the default policy.
func (p DefaultAutoToolPolicy) decideShellTransform(ctx stdcontext.Context, request ToolDecisionRequest) (ToolDecision, bool) {
	// Don't transform if we're already inside a transformed execution. This
	// is the policy-side half of the one-level transform guard; the runtime
	// side lives in ToolRuntime.executeTransformedTool.
	if transformDepthFromCtx(ctx) >= 1 {
		return ToolDecision{}, false
	}

	command, cwd, ok := parseShellInput(request.NormalizedInput, request.WorkspaceRoot)
	if !ok {
		return ToolDecision{}, false
	}

	tier := shelltool.ClassifyShellCommand(command, cwd)

	switch tier {
	case shelltool.ShellTierOpenWorldSystem:
		return ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourceHeadlessDefault,
			Reason: "shell command reaches outside the workspace or affects the system; use apply_patch directly for file writes",
		}, true

	case shelltool.ShellTierWorkspaceMutation:
		newToolName, newInput, transformed, reason := TransformShellToFileChain(command, cwd, request.WorkspaceRoot)
		if transformed {
			inputJSON, err := json.Marshal(newInput)
			if err != nil {
				return ToolDecision{
					Action: ToolDecisionDeny,
					Source: ToolDecisionSourcePolicyError,
					Reason: fmt.Sprintf("failed to encode transformed shell input: %v", err),
				}, true
			}
			return ToolDecision{
				Action:          ToolDecisionTransform,
				Source:          ToolDecisionSourceTransform,
				Reason:          "shell file-write rewritten to apply_patch",
				UpdatedToolName: newToolName,
				UpdatedInput:    string(inputJSON),
			}, true
		}
		return ToolDecision{
			Action: ToolDecisionDeny,
			Source: ToolDecisionSourceHeadlessDefault,
			Reason: firstNonEmpty(reason, "shell command mutates workspace files; use apply_patch directly"),
		}, true
	}

	// inspect_read_only: fall through to the default policy.
	return ToolDecision{}, false
}

// parseShellInput extracts the command and effective cwd from a shell tool's
// normalized input. The cwd is computed from the optional workdir field and
// the workspace root.
func parseShellInput(normalizedInput string, workspaceRoot string) (command string, cwd string, ok bool) {
	normalizedInput = strings.TrimSpace(normalizedInput)
	if normalizedInput == "" {
		return "", "", false
	}
	var parsed struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	if err := json.Unmarshal([]byte(normalizedInput), &parsed); err != nil {
		return "", "", false
	}
	command = strings.TrimSpace(parsed.Command)
	if command == "" {
		return "", "", false
	}
	workdir := strings.TrimSpace(parsed.Workdir)
	switch {
	case workdir == "":
		cwd = workspaceRoot
	case filepath.IsAbs(workdir):
		cwd = workdir
	case workspaceRoot != "":
		cwd = filepath.Join(workspaceRoot, workdir)
	default:
		cwd = workdir
	}
	return command, cwd, true
}

// hasCompoundOperator reports whether command contains a shell compound
// operator (&&, ||, ;, |) outside of quoted strings.
func hasCompoundOperator(command string) bool {
	inSingle := false
	inDouble := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case inSingle || inDouble:
			continue
		case c == ';':
			return true
		case c == '&' && i+1 < len(command) && command[i+1] == '&':
			return true
		case c == '|' && i+1 < len(command) && command[i+1] == '|':
			return true
		case c == '|':
			return true
		}
	}
	return false
}

// parseEchoRedirect parses `echo <content> (>|>>) <path>` and returns the
// content, path, and whether it's append mode (>>). It only handles a single
// content token and a single path token, each of which may be quoted. Multiple
// unquoted content tokens (e.g. `echo hello world > file`) are rejected to
// keep the parser conservative.
func parseEchoRedirect(command string) (content string, path string, appendMode bool, ok bool) {
	command = strings.TrimSpace(command)
	if !strings.HasPrefix(command, "echo") {
		return "", "", false, false
	}
	rest := command[4:]
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return "", "", false, false
	}
	rest = strings.TrimLeft(rest, " \t")

	redirectIdx, redirectLen, found := findRedirectOp(rest)
	if !found {
		return "", "", false, false
	}

	contentPart := strings.TrimSpace(rest[:redirectIdx])
	pathPart := strings.TrimSpace(rest[redirectIdx+redirectLen:])

	contentStr, ok := unquoteToken(contentPart)
	if !ok {
		return "", "", false, false
	}

	pathStr, ok := unquoteToken(pathPart)
	if !ok || pathStr == "" {
		return "", "", false, false
	}

	return contentStr, pathStr, redirectLen == 2, true
}

// findRedirectOp finds the first > or >> operator outside of quotes. It
// returns the index and length (1 or 2) of the operator.
func findRedirectOp(s string) (idx int, length int, found bool) {
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case inSingle || inDouble:
			continue
		case c == '>':
			if i+1 < len(s) && s[i+1] == '>' {
				return i, 2, true
			}
			return i, 1, true
		}
	}
	return 0, 0, false
}

// unquoteToken removes surrounding quotes from a token. If the token is
// unquoted and contains whitespace, it returns ok=false (we only accept a
// single unquoted token).
func unquoteToken(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", true
	}
	if len(token) >= 2 {
		if token[0] == '"' && token[len(token)-1] == '"' {
			return token[1 : len(token)-1], true
		}
		if token[0] == '\'' && token[len(token)-1] == '\'' {
			return token[1 : len(token)-1], true
		}
	}
	if strings.ContainsAny(token, " \t") {
		return "", false
	}
	return token, true
}

func resolveTransformPath(path string, cwd string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if cwd == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(cwd, path))
}

func isPathWithinRoot(path string, root string) bool {
	if root == "" {
		return false
	}
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

// buildAddFilePatch builds a Codex-style Add File patch that creates or
// overwrites relPath with content. apply_patch's Add File overwrites existing
// files, so this is used for both `>` and `>>` (after pre-composing the
// existing content for append mode).
func buildAddFilePatch(relPath string, content string) string {
	var b strings.Builder
	b.WriteString("*** Begin Patch\n")
	b.WriteString("*** Add File: " + relPath + "\n")
	// Strip a single trailing newline; the +lines reconstruct it.
	trimmed := strings.TrimSuffix(content, "\n")
	for _, line := range strings.Split(trimmed, "\n") {
		b.WriteString("+" + line + "\n")
	}
	b.WriteString("*** End Patch")
	return b.String()
}
