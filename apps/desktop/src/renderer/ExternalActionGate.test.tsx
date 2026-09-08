import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { ExternalActionGate } from "./ExternalActionGate";

describe("ExternalActionGate", () => {
  it("renders a safe HTTPS link and fixed QR frame without narrative data", () => {
    const html = renderToString(
      <ExternalActionGate
        interaction={{
          id: "ix-1",
          kind: "external",
          title: "钉钉授权",
          safeMessage: "完成钉钉授权后继续",
          verificationUrl: "https://login.dingtalk.com/oauth2/auth",
          provider: "dingtalk-workspace",
          recoveries: ["open_url", "confirm_done", "cancel"],
        }}
        onConfirmDone={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    expect(html).toContain("external-action-gate");
    expect(html).toContain("钉钉授权");
    expect(html).toContain("https://login.dingtalk.com/oauth2/auth");
    expect(html).toContain("cli-auth-qr-frame");
    expect(html).toContain("我已完成");
    expect(html).toContain("取消");
  });

  it("renders precise ACP option buttons and calls back with option id", () => {
    const onResolveACPOption = vi.fn();
    const onCancel = vi.fn();
    const html = renderToString(
      <ExternalActionGate
        interaction={{
          id: "perm-1",
          kind: "acp-permission",
          title: "Claude Code wants to edit files",
          safeMessage: "Claude Code 需要你的确认",
          provider: "claude-code",
          recoveries: ["acp_option", "cancel"],
          options: [
            { optionId: "allow", name: "Allow" },
            { optionId: "deny", name: "Deny" },
          ],
        }}
        onConfirmDone={vi.fn()}
        onCancel={onCancel}
        onResolveACPOption={onResolveACPOption}
      />,
    );
    expect(html).toContain("Allow");
    expect(html).toContain("Deny");
    expect(html).toContain("取消");
    expect(html).not.toContain("cli-auth-qr-frame");
  });

  it("renders an MCP elicitation flat form with defaults", () => {
    const html = renderToString(
      <ExternalActionGate
        interaction={{
          id: "mcp-elicit-1",
          kind: "mcp-elicitation",
          title: "MCP 请求确认",
          safeMessage: "Choose where to save the file",
          provider: "plugin__openai-developers__local",
          recoveries: ["submit_form", "cancel"],
          formSchema: {
            type: "object",
            properties: {
              targetPath: { type: "string", title: "Save location", default: ".env.local" },
            },
            required: ["targetPath"],
          },
        }}
        onConfirmDone={vi.fn()}
        onCancel={vi.fn()}
        onSubmitForm={vi.fn()}
      />,
    );
    expect(html).toContain("mcp-elicitation-form");
    expect(html).toContain("Save location");
    expect(html).toContain(".env.local");
    expect(html).toContain("确认");
    expect(html).toContain("取消");
    expect(html).not.toContain("cli-auth-qr-frame");
  });
});
