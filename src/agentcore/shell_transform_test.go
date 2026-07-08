package agentcore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransformShellEchoWriteToApplyPatch(t *testing.T) {
	workspace := t.TempDir()
	cwd := workspace

	toolName, input, ok, reason := TransformShellToFileChain(
		`echo "hello" > ./file.txt`, cwd, workspace)

	if !ok {
		t.Fatalf("expected transform to succeed, got reason: %s", reason)
	}
	if toolName != "apply_patch" {
		t.Fatalf("expected apply_patch, got %s", toolName)
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "*** Add File: file.txt") {
		t.Fatalf("expected Add File for file.txt, got: %s", patch)
	}
	if !strings.Contains(patch, "+hello") {
		t.Fatalf("expected +hello in patch, got: %s", patch)
	}
	if !strings.Contains(patch, "*** Begin Patch") || !strings.Contains(patch, "*** End Patch") {
		t.Fatalf("expected patch markers, got: %s", patch)
	}
}

func TestTransformShellEchoAppendToExistingFile(t *testing.T) {
	workspace := t.TempDir()
	existing := filepath.Join(workspace, "file.txt")
	if err := os.WriteFile(existing, []byte("first\n"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	toolName, input, ok, reason := TransformShellToFileChain(
		`echo "second" >> ./file.txt`, workspace, workspace)

	if !ok {
		t.Fatalf("expected transform to succeed, got reason: %s", reason)
	}
	if toolName != "apply_patch" {
		t.Fatalf("expected apply_patch, got %s", toolName)
	}
	patch, _ := input["patch"].(string)
	// Append pre-composes existing content; both lines should appear as +lines.
	if !strings.Contains(patch, "+first") {
		t.Fatalf("expected +first in append patch, got: %s", patch)
	}
	if !strings.Contains(patch, "+second") {
		t.Fatalf("expected +second in append patch, got: %s", patch)
	}
}

func TestTransformShellEchoAppendToMissingFileCreatesFile(t *testing.T) {
	workspace := t.TempDir()

	toolName, input, ok, reason := TransformShellToFileChain(
		`echo "hello" >> ./missing.txt`, workspace, workspace)

	if !ok {
		t.Fatalf("expected append to missing file to transform as create, got reason: %s", reason)
	}
	if toolName != "apply_patch" {
		t.Fatalf("expected apply_patch, got %s", toolName)
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "+hello") {
		t.Fatalf("expected +hello in patch, got: %s", patch)
	}
	if strings.Contains(patch, "first") {
		t.Fatalf("expected no pre-existing content in create-from-append patch, got: %s", patch)
	}
}

func TestTransformShellRejectsPathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()

	_, _, ok, reason := TransformShellToFileChain(
		`echo "hello" > /etc/passwd`, workspace, workspace)

	if ok {
		t.Fatalf("expected transform to reject /etc/passwd")
	}
	if !strings.Contains(reason, "outside the workspace") {
		t.Fatalf("expected reason to mention outside workspace, got: %s", reason)
	}
}

func TestTransformShellRejectsSedInPlace(t *testing.T) {
	workspace := t.TempDir()

	_, _, ok, reason := TransformShellToFileChain(
		`sed -i 's/a/b/' file.txt`, workspace, workspace)

	if ok {
		t.Fatalf("expected sed -i to not be transformable")
	}
	if !strings.Contains(reason, "apply_patch") {
		t.Fatalf("expected reason to suggest apply_patch, got: %s", reason)
	}
}

func TestTransformShellRejectsOpenWorldCurl(t *testing.T) {
	workspace := t.TempDir()

	_, _, ok, reason := TransformShellToFileChain(
		`curl http://example.com > file`, workspace, workspace)

	if ok {
		t.Fatalf("expected curl to not be transformable")
	}
	if !strings.Contains(reason, "open-world") {
		t.Fatalf("expected reason to mention open-world, got: %s", reason)
	}
}

func TestTransformShellRejectsReadOnlyCommand(t *testing.T) {
	workspace := t.TempDir()

	_, _, ok, _ := TransformShellToFileChain(
		`ls -la`, workspace, workspace)

	if ok {
		t.Fatalf("expected ls to not be transformable")
	}
}

func TestTransformShellRejectsCompoundCommand(t *testing.T) {
	workspace := t.TempDir()

	_, _, ok, reason := TransformShellToFileChain(
		`echo "hello" > file.txt && echo "world" > other.txt`, workspace, workspace)

	if ok {
		t.Fatalf("expected compound command to not be transformable")
	}
	if !strings.Contains(reason, "compound") {
		t.Fatalf("expected reason to mention compound, got: %s", reason)
	}
}

func TestTransformShellHandlesUnquotedContent(t *testing.T) {
	workspace := t.TempDir()

	toolName, input, ok, reason := TransformShellToFileChain(
		`echo hello > file.txt`, workspace, workspace)

	if !ok {
		t.Fatalf("expected transform to succeed, got reason: %s", reason)
	}
	if toolName != "apply_patch" {
		t.Fatalf("expected apply_patch, got %s", toolName)
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "+hello") {
		t.Fatalf("expected +hello in patch, got: %s", patch)
	}
}

func TestTransformShellHandlesSingleQuotedContent(t *testing.T) {
	workspace := t.TempDir()

	_, input, ok, _ := TransformShellToFileChain(
		`echo 'hello world' > file.txt`, workspace, workspace)

	if !ok {
		t.Fatalf("expected single-quoted content to transform")
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "+hello world") {
		t.Fatalf("expected +hello world in patch, got: %s", patch)
	}
}

func TestTransformShellHandlesQuotedPath(t *testing.T) {
	workspace := t.TempDir()

	_, input, ok, _ := TransformShellToFileChain(
		`echo "hello" > "file.txt"`, workspace, workspace)

	if !ok {
		t.Fatalf("expected quoted path to transform")
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "*** Add File: file.txt") {
		t.Fatalf("expected Add File for file.txt, got: %s", patch)
	}
}

func TestTransformShellRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()

	_, _, ok, reason := TransformShellToFileChain(
		`echo "hello" > `+filepath.Join(outside, "file.txt"), workspace, workspace)

	if ok {
		t.Fatalf("expected transform to reject path outside workspace")
	}
	if !strings.Contains(reason, "outside the workspace") {
		t.Fatalf("expected reason to mention outside workspace, got: %s", reason)
	}
}

func TestTransformShellHandlesNestedRelativePath(t *testing.T) {
	workspace := t.TempDir()

	_, input, ok, _ := TransformShellToFileChain(
		`echo "hello" > ./nested/deep/file.txt`, workspace, workspace)

	if !ok {
		t.Fatalf("expected nested relative path to transform")
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "*** Add File: nested/deep/file.txt") {
		t.Fatalf("expected Add File for nested/deep/file.txt, got: %s", patch)
	}
}

func TestTransformShellCatHeredocWriteToApplyPatch(t *testing.T) {
	workspace := t.TempDir()
	command := "cat <<'EOF' > report.md\nline one\nline two\nEOF"

	toolName, input, ok, reason := TransformShellToFileChain(command, workspace, workspace)

	if !ok {
		t.Fatalf("expected heredoc transform to succeed, got reason: %s", reason)
	}
	if toolName != "apply_patch" {
		t.Fatalf("expected apply_patch, got %s", toolName)
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "*** Add File: report.md") {
		t.Fatalf("expected Add File for report.md, got: %s", patch)
	}
	if !strings.Contains(patch, "+line one") || !strings.Contains(patch, "+line two") {
		t.Fatalf("expected heredoc content lines in patch, got: %s", patch)
	}
}

func TestTransformShellCatHeredocRedirectBeforeDelimiter(t *testing.T) {
	workspace := t.TempDir()
	command := "cat > report.md <<EOF\nhello\nEOF"

	_, input, ok, reason := TransformShellToFileChain(command, workspace, workspace)

	if !ok {
		t.Fatalf("expected heredoc transform to succeed, got reason: %s", reason)
	}
	patch, _ := input["patch"].(string)
	if !strings.Contains(patch, "*** Add File: report.md") || !strings.Contains(patch, "+hello") {
		t.Fatalf("expected report.md heredoc patch, got: %s", patch)
	}
}
