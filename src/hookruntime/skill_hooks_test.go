package hookruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompileActivatedSkillHooksAnchorsCommandInsidePackage(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "scripts", "check")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"BeforeFinish": []any{map[string]any{"matcher": map[string]any{"toolNames": []any{}}, "hooks": []any{map[string]any{"id": "verify", "type": "command", "command": []any{"scripts/check"}, "timeoutMs": 1000, "maxOutputBytes": 1024, "onFailure": "continue"}}}}}
	hooks, err := CompileActivatedSkillHooks("review", root, filepath.Join(root, "SKILL.md"), SourceUser, raw)
	resolvedScript, _ := filepath.EvalSymlinks(script)
	if err != nil || len(hooks) != 1 || hooks[0].Command.Command[0] != resolvedScript || hooks[0].StableID != "skill:review:verify" {
		t.Fatalf("compiled=%+v err=%v", hooks, err)
	}
	if err := NewDispatcher(EmptySnapshot(), "").RegisterActivatedSkillHooks(hooks); err != nil {
		t.Fatal(err)
	}
}

func TestCompileActivatedSkillHooksRejectsPackageEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "escape")
	if err := os.WriteFile(outside, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"RunEnd": []any{map[string]any{"matcher": map[string]any{"toolNames": []any{}}, "hooks": []any{map[string]any{"id": "escape", "type": "command", "command": []any{outside}, "timeoutMs": 1000, "maxOutputBytes": 1024, "onFailure": "continue"}}}}}
	if _, err := CompileActivatedSkillHooks("escape", root, filepath.Join(root, "SKILL.md"), SourceUser, raw); err == nil {
		t.Fatal("absolute command escaped package root")
	}
}
