package shelltool

import "testing"

func TestClassifyShellCommand(t *testing.T) {
	cwd := t.TempDir()

	cases := []struct {
		name    string
		command string
		want    ShellTier
	}{
		// inspect_read_only
		{"ls -la", "ls -la", ShellTierInspectReadOnly},
		{"cat file", "cat file.txt", ShellTierInspectReadOnly},
		{"git status", "git status", ShellTierInspectReadOnly},
		{"rg pattern", `rg "pattern" .`, ShellTierInspectReadOnly},

		// local_cli
		{"dws search", `dws aitable base search --query "DVF" --format json`, ShellTierLocalCLI},
		{"gh issue list", "gh issue list --limit 10", ShellTierLocalCLI},
		{"obsidian search", `obsidian note search "ERP"`, ShellTierLocalCLI},

		// workspace_mutation
		{"mkdir", "mkdir newdir", ShellTierWorkspaceMutation},
		{"touch", "touch file.txt", ShellTierWorkspaceMutation},
		{"rm", "rm file.txt", ShellTierWorkspaceMutation},
		{"git commit", `git commit -m "msg"`, ShellTierWorkspaceMutation},
		{"npm install", "npm install", ShellTierWorkspaceMutation},

		// open_world_system
		{"curl", "curl https://example.com", ShellTierOpenWorldSystem},
		{"git push", "git push", ShellTierOpenWorldSystem},
		{"sudo apt", "sudo apt update", ShellTierOpenWorldSystem},

		// compound commands — highest segment wins
		{"ls && rm", "ls && rm file", ShellTierWorkspaceMutation},
		{"local cli && rm", "dws schema && rm file", ShellTierWorkspaceMutation},
		{"cat | curl", "cat file | curl -X POST -d @- url", ShellTierOpenWorldSystem},

		// redirects
		{"redirect to system path", `echo "hi" > /etc/passwd`, ShellTierOpenWorldSystem},
		{"redirect to workspace path", `echo "hi" > ./local.txt`, ShellTierWorkspaceMutation},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyShellCommand(tc.command, cwd)
			if got != tc.want {
				t.Errorf("ClassifyShellCommand(%q, %q) = %q, want %q",
					tc.command, cwd, got, tc.want)
			}
		})
	}
}

func TestClassifyShellCommandEmpty(t *testing.T) {
	if got := ClassifyShellCommand("", "/tmp"); got != ShellTierInspectReadOnly {
		t.Errorf("ClassifyShellCommand(\"\", ...) = %q, want %q", got, ShellTierInspectReadOnly)
	}
	if got := ClassifyShellCommand("   ", "/tmp"); got != ShellTierInspectReadOnly {
		t.Errorf("ClassifyShellCommand(\"   \", ...) = %q, want %q", got, ShellTierInspectReadOnly)
	}
}

func TestClassifyShellCommandHelperPredicates(t *testing.T) {
	cwd := t.TempDir()

	if !IsInspectReadOnlyCommand("ls", cwd) {
		t.Errorf("IsInspectReadOnlyCommand(\"ls\", cwd) = false, want true")
	}
	if !IsWorkspaceMutationCommand("mkdir foo", cwd) {
		t.Errorf("IsWorkspaceMutationCommand(\"mkdir foo\", cwd) = false, want true")
	}
	if !IsLocalCLICommand("dws schema", cwd) {
		t.Errorf("IsLocalCLICommand(\"dws schema\", cwd) = false, want true")
	}
	if !IsOpenWorldCommand("curl https://example.com", cwd) {
		t.Errorf("IsOpenWorldCommand(\"curl ...\", cwd) = false, want true")
	}
}
