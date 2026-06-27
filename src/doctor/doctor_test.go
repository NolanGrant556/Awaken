package doctor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewReportAggregatesWorstStatus(t *testing.T) {
	report := NewReport([]CheckResult{
		{Status: StatusOK},
		{Status: StatusInfo},
		{Status: StatusWarning},
	})

	if report.Summary.Status != StatusWarning {
		t.Fatalf("expected warning aggregate, got %s", report.Summary.Status)
	}
	if report.Summary.OK != 1 || report.Summary.Info != 1 || report.Summary.Warnings != 1 || report.Summary.Failures != 0 {
		t.Fatalf("unexpected summary counts: %+v", report.Summary)
	}

	report = NewReport(append(report.Checks, CheckResult{Status: StatusFailure}))
	if report.Summary.Status != StatusFailure {
		t.Fatalf("expected failure aggregate, got %s", report.Summary.Status)
	}
	if report.Summary.Failures != 1 {
		t.Fatalf("expected one failure, got %+v", report.Summary)
	}
}

func TestRedactRemovesProviderSecrets(t *testing.T) {
	input := strings.Join([]string{
		"LLM_API_KEY=sk-1234567890abcdef",
		"OPENAI_TOKEN='token-value-123456'",
		"Authorization: Bearer abcdefghijklmnop",
		"safe=model-id",
	}, "\n")

	redacted := Redact(input)
	for _, secret := range []string{"1234567890abcdef", "token-value-123456", "abcdefghijklmnop"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("expected %q to be redacted from %q", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "LLM_API_KEY=<redacted>") {
		t.Fatalf("expected API key assignment to be redacted, got %q", redacted)
	}
	if !strings.Contains(redacted, "safe=model-id") {
		t.Fatalf("expected non-secret text to remain, got %q", redacted)
	}
}

func TestProviderConfigDetectionUsesEnvAndFiles(t *testing.T) {
	files := fakeFiles{
		files: map[string]string{
			"/cfg/.env": "LLM_API_KEY=sk-file-secret-123456\n",
			"/cfg/config.toml": `[llm]
base_url = "https://api.example.com/v1"
model_id = "test-model"
`,
		},
	}
	runner := NewRunner(Options{
		WorkspaceRoot: "/repo",
		ConfigDir:     "/cfg",
		Env: fakeEnv{
			values: map[string]string{
				"LLM_API_KEY": "sk-env-secret-123456",
			},
			home: "/home/test",
			cwd:  "/repo",
		},
		Files:    files,
		Commands: &fakeCommands{},
	})

	result := runner.checkProviderConfig()
	if result.Status != StatusOK {
		t.Fatalf("expected provider config to pass, got %+v", result)
	}
	if strings.Contains(result.RedactedOutput, "sk-env-secret") || strings.Contains(result.RedactedOutput, "sk-file-secret") {
		t.Fatalf("expected provider output to be redacted, got %q", result.RedactedOutput)
	}
	if !strings.Contains(result.RedactedOutput, "LLM_API_KEY=<redacted>") {
		t.Fatalf("expected redacted API key output, got %q", result.RedactedOutput)
	}
}

func TestProviderConfigDetectionWarnsForBlankTemplate(t *testing.T) {
	runner := NewRunner(Options{
		WorkspaceRoot: "/repo",
		ConfigDir:     "/cfg",
		Env: fakeEnv{
			values: map[string]string{},
			home:   "/home/test",
			cwd:    "/repo",
		},
		Files: fakeFiles{
			files: map[string]string{
				"/cfg/.env":        "LLM_API_KEY=\n",
				"/cfg/config.toml": "[llm]\nbase_url = \"https://api.example.com/v1\"\n",
			},
		},
		Commands: &fakeCommands{},
	})

	result := runner.checkProviderConfig()
	if result.Status != StatusWarning {
		t.Fatalf("expected provider warning, got %+v", result)
	}
	if len(result.Remediation) != 2 {
		t.Fatalf("expected API key and model remediation hints, got %+v", result.Remediation)
	}
}

func TestGoVersionCheckFailsWhenCommandReportsOlderVersion(t *testing.T) {
	commands := fakeCommands{
		results: map[string]CommandResult{
			"go version": {ExitCode: 0, Stdout: "go version go1.24.1 darwin/arm64\n"},
		},
	}
	runner := NewRunner(Options{
		WorkspaceRoot: "/repo",
		Env:           fakeEnv{home: "/home/test", cwd: "/repo"},
		Files: fakeFiles{
			files: map[string]string{
				"/repo/go.mod": "module awaken\n\ngo 1.25.2\n",
			},
		},
		Commands: &commands,
	})

	result := runner.checkGoVersion(context.Background())
	if result.Status != StatusFailure {
		t.Fatalf("expected old Go version to fail, got %+v", result)
	}
	if !strings.Contains(strings.Join(result.Remediation, "\n"), "1.25.2") {
		t.Fatalf("expected remediation to mention required version, got %+v", result.Remediation)
	}
}

func TestNodeNPMCheckUsesCommandProbeOutcomes(t *testing.T) {
	commands := fakeCommands{
		results: map[string]CommandResult{
			"node --version": {ExitCode: 0, Stdout: "v25.0.0\n"},
			"npm --version":  {ExitCode: -1, Err: errors.New("executable file not found")},
		},
	}
	runner := NewRunner(Options{
		WorkspaceRoot: "/repo",
		Env:           fakeEnv{home: "/home/test", cwd: "/repo"},
		Files:         fakeFiles{files: map[string]string{}},
		Commands:      &commands,
	})

	result := runner.checkNodeNPM(context.Background())
	if result.Status != StatusFailure {
		t.Fatalf("expected missing npm to fail, got %+v", result)
	}
	if !strings.Contains(strings.Join(result.Details, "\n"), "npm --version") {
		t.Fatalf("expected command details to mention npm, got %+v", result.Details)
	}
}

func TestGitWorktreeCheckWarnsOnDirtyStatus(t *testing.T) {
	commands := fakeCommands{
		results: map[string]CommandResult{
			"git status --porcelain": {ExitCode: 0, Stdout: " M src/doctor/doctor.go\n?? scratch.txt\n"},
		},
	}
	runner := NewRunner(Options{
		WorkspaceRoot: "/repo",
		Env:           fakeEnv{home: "/home/test", cwd: "/repo"},
		Files:         fakeFiles{files: map[string]string{}},
		Commands:      &commands,
	})

	result := runner.checkGitWorktree(context.Background())
	if result.Status != StatusWarning {
		t.Fatalf("expected dirty git status to warn, got %+v", result)
	}
	if !strings.Contains(strings.Join(result.Details, "\n"), "scratch.txt") {
		t.Fatalf("expected dirty file details, got %+v", result.Details)
	}
	if len(commands.calls) != 1 || commands.calls[0].Dir != "/repo" {
		t.Fatalf("expected git command to run in workspace root, got %+v", commands.calls)
	}
}

type fakeEnv struct {
	values map[string]string
	home   string
	cwd    string
}

func (f fakeEnv) LookupEnv(key string) (string, bool) {
	value, ok := f.values[key]
	return value, ok
}

func (f fakeEnv) HomeDir() (string, error) {
	return f.home, nil
}

func (f fakeEnv) WorkingDir() (string, error) {
	return f.cwd, nil
}

type fakeFiles struct {
	files  map[string]string
	errors map[string]error
}

func (f fakeFiles) Exists(path string) (bool, error) {
	if err := f.errors[path]; err != nil {
		return false, err
	}
	_, ok := f.files[path]
	if ok {
		return true, nil
	}
	prefix := path + string(filepath.Separator)
	for filePath := range f.files {
		if strings.HasPrefix(filePath, prefix) {
			return true, nil
		}
	}
	return false, nil
}

func (f fakeFiles) ReadFile(path string) (string, error) {
	if err := f.errors[path]; err != nil {
		return "", err
	}
	content, ok := f.files[path]
	if !ok {
		return "", errors.New("file not found")
	}
	return content, nil
}

type fakeCommands struct {
	results map[string]CommandResult
	calls   []CommandSpec
}

func (f *fakeCommands) Run(ctx context.Context, spec CommandSpec) CommandResult {
	_ = ctx
	f.calls = append(f.calls, spec)
	if result, ok := f.results[spec.Display()]; ok {
		return result
	}
	return CommandResult{ExitCode: -1, Err: errors.New("unexpected command: " + spec.Display())}
}
