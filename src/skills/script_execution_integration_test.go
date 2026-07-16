package skills

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"awaken/src/tool"
	bashtool "awaken/src/tools/bash"
)

func TestInstalledSkillPythonNodeAndShellScriptsUseCurrentUserShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX sh commands")
	}
	for _, executable := range []string{"python3", "node", "sh"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skipf("%s is unavailable: %v", executable, err)
		}
	}

	packageRoot := filepath.Join(t.TempDir(), "script-fixture")
	if err := os.MkdirAll(filepath.Join(packageRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"SKILL.md": `---
name: script-fixture
description: Exercise installed package scripts through the current-user Shell.
---
Run only the package script needed by the task.
`,
		"scripts/create.py":  "from pathlib import Path\nPath('python-output.txt').write_text('python')\n",
		"scripts/create.mjs": "import { writeFileSync } from 'node:fs';\nwriteFileSync('node-output.txt', 'node');\n",
		"scripts/create.sh":  "#!/bin/sh\nprintf shell > shell-output.txt\n",
	}
	for relative, content := range files {
		if err := os.WriteFile(filepath.Join(packageRoot, relative), []byte(content), 0o755); err != nil {
			t.Fatalf("write %s: %v", relative, err)
		}
	}
	pkg, err := PackageFromPath(packageRoot)
	if err != nil {
		t.Fatalf("package Skill fixture: %v", err)
	}
	workspace := t.TempDir()
	installed, err := InstallSkillPackage(InstallRequest{
		Package: pkg, Scope: string(SourceProject), ProjectRoot: workspace, OverwritePolicy: "abort",
	})
	if err != nil || installed.Skill == nil || installed.Skill.ID != "script-fixture" {
		t.Fatalf("install Skill fixture: result=%+v err=%v", installed, err)
	}

	permissionContext := tool.NewPermissionContext(workspace, []string{workspace})
	ctx := tool.ContextWithPermissionContext(context.Background(), permissionContext)
	commands := []struct {
		command string
		output  string
		want    string
	}{
		{command: "python3 .awaken/skills/script-fixture/scripts/create.py", output: "python-output.txt", want: "python"},
		{command: "node .awaken/skills/script-fixture/scripts/create.mjs", output: "node-output.txt", want: "node"},
		{command: "sh .awaken/skills/script-fixture/scripts/create.sh", output: "shell-output.txt", want: "shell"},
	}
	for _, testCase := range commands {
		input, marshalErr := json.Marshal(map[string]any{
			"command": testCase.command, "timeout": 30_000,
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		outcome, executeErr := (&bashtool.Tool{}).ExecuteOutcome(ctx, string(input), nil)
		if executeErr != nil || outcome.Status != tool.OutcomeSucceeded || outcome.ExitCode == nil || *outcome.ExitCode != 0 {
			t.Fatalf("execute %q: outcome=%+v err=%v", testCase.command, outcome, executeErr)
		}
		if len(outcome.Changes) != 0 {
			t.Fatalf("direct Skill script execution fabricated typed mutation facts: %+v", outcome)
		}
		content, readErr := os.ReadFile(filepath.Join(workspace, testCase.output))
		if readErr != nil || string(content) != testCase.want {
			t.Fatalf("script %q output=%q err=%v", testCase.command, content, readErr)
		}
	}
}
