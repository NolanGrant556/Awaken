package plugins

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"awaken/src/skills"
	"testing"

	"awaken/src/plugins/ir"
)

func TestOfficialOutlookPluginDeclaresHostConnectorForEverySkill(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "apps", "desktop", "resources", "builtin-plugins", "awaken-outlook"))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(t.TempDir())
	validated, err := store.Validate(context.Background(), Source{Kind: SourceLocalPath, Path: root, DialectHint: ir.DialectAgentPlugins})
	if err != nil {
		t.Fatal(err)
	}
	if !validated.Valid {
		t.Fatalf("validation = %+v", validated)
	}
	if validated.Name != "awaken-outlook" || len(validated.Setup) != 1 || validated.Setup[0].Kind != ir.SetupHostConnector {
		t.Fatalf("validation = %+v", validated)
	}
	if validated.Version == "" {
		t.Fatalf("Outlook plugin version = %q", validated.Version)
	}
	requirement := validated.Setup[0]
	if requirement.ConnectorID != "awaken.microsoft.outlook" || len(requirement.ComponentIDs) != 10 {
		t.Fatalf("setup = %+v", requirement)
	}
	mcpCount := 0
	skillCount := 0
	chineseNames := map[string]string{
		"skill/skills/attachment-management": "邮件附件处理",
		"skill/skills/inbox-triage":          "收件箱整理",
		"skill/skills/mailbox-organization":  "邮箱文件夹与分类",
		"skill/skills/outlook-email":         "Outlook 邮件",
		"skill/skills/reply-drafting":        "邮件回复草拟",
		"skill/skills/scheduled-mail":        "定时发送邮件",
		"skill/skills/shared-mailboxes":      "共享邮箱",
		"skill/skills/subscription-cleanup":  "订阅邮件清理",
		"skill/skills/task-extraction":       "邮件任务提取",
	}
	for _, component := range validated.Components {
		if component.Kind == ir.KindMCPServer {
			mcpCount++
			if component.MCP == nil || component.MCP.HostConnectorID != "awaken.microsoft.outlook" || component.MCP.ShortName != "awaken-outlook" {
				t.Fatalf("Outlook MCP component = %+v", component)
			}
			if !containsOutlookTestString(requirement.ComponentIDs, component.LogicalID) {
				t.Fatalf("MCP %s is not gated by Outlook account setup", component.LogicalID)
			}
			continue
		}
		if component.Kind != ir.KindSkill {
			continue
		}
		skillCount++
		if component.DisplayName != chineseNames[component.LogicalID] || component.Description == "" {
			t.Fatalf("Outlook Skill copy = %+v", component)
		}
		if !containsOutlookTestString(requirement.ComponentIDs, component.LogicalID) {
			t.Fatalf("skill %s is not gated by Outlook account setup", component.LogicalID)
		}
	}
	if mcpCount != 1 {
		t.Fatalf("Outlook MCP component count = %d", mcpCount)
	}
	if skillCount != 9 {
		t.Fatalf("Outlook Skill component count = %d", skillCount)
	}
}

func TestOfficialOutlookWorkflowResourcesSurviveInstallAndExport(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "apps", "desktop", "resources", "builtin-plugins", "awaken-outlook"))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(t.TempDir())
	validation, err := store.Validate(context.Background(), Source{Kind: SourceLocalPath, Path: root})
	if err != nil || !validation.Valid {
		t.Fatalf("validation=%+v err=%v", validation, err)
	}
	installed, err := store.Install(context.Background(), InstallRequest{
		Source: Source{Kind: SourceLocalPath, Path: root}, ExpectedDigest: validation.Digest,
	})
	if err != nil {
		t.Fatal(err)
	}
	exported := filepath.Join(t.TempDir(), "exported")
	if _, err := store.Export("awaken-outlook", exported); err != nil {
		t.Fatal(err)
	}
	for _, packageRoot := range []string{installed.Plugin.Root, exported} {
		definitions := skills.LoadDir(filepath.Join(packageRoot, "skills"), skills.SourceProject)
		definition := definitions["outlook-email"]
		if definition == nil {
			t.Fatal("outlook-email is missing from the installed catalog")
		}
		inventory, err := skills.ListSkillPackageResources(definition)
		if err != nil || inventory.Truncated {
			t.Fatalf("resource inventory=%+v err=%v", inventory, err)
		}
		for _, name := range []string{"读取与对象.md", "发送与确认.md"} {
			rel := filepath.Join("skills", "outlook-email", "references", name)
			original, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(filepath.Join(packageRoot, rel))
			if err != nil || !bytes.Equal(original, actual) {
				t.Fatalf("workflow resource changed or missing: %s, err=%v", rel, err)
			}
			found := false
			for _, entry := range inventory.Entries {
				if entry.Path == "references/"+name && entry.Kind == "file" {
					found = true
				}
			}
			if !found {
				t.Fatalf("workflow resource is not discoverable: %s", name)
			}
		}
	}
}

func containsOutlookTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
