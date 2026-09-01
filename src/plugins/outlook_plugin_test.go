package plugins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	if validated.Version != "1.4.0" {
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

func TestOfficialOutlookSkillsDefineDraftFirstConfirmationProtocol(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "apps", "desktop", "resources", "builtin-plugins", "awaken-outlook"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path     string
		required []string
	}{
		{path: "outlook-email", required: []string{"提问用户", "停止等待", "发送摘要", `{"to":["person@example.com"]}`, "结果未知时不要自动再发", "get_messages_batch", "nextCursor", "find_mail_folder"}},
		{path: "reply-drafting", required: []string{"原邮件发件人", "提问用户", "停止等待", "只调用一次", "额外抄送"}},
		{path: "scheduled-mail", required: []string{"提问用户", "停止等待", "不得调用 `send_draft`", "already_absent"}},
		{path: "shared-mailboxes", required: []string{"发件邮箱为该共享邮箱", "提问用户", "停止等待"}},
		{path: "attachment-management", required: []string{"发送摘要", "提问用户", "停止等待"}},
	} {
		data, err := os.ReadFile(filepath.Join(root, "skills", test.path, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, required := range test.required {
			if !strings.Contains(content, required) {
				t.Errorf("%s is missing %q", test.path, required)
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
