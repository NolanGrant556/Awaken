import { readStyleSource } from "../style-test-source";
// @vitest-environment jsdom
/// <reference types="node" />
import {
  AwakenPluginConsole,
  type AwakenPluginConsoleComponent,
  type AwakenPluginConsoleDetail,
  type AwakenPluginSetupItem,
} from "@awaken/design-system";
import { resolve } from "node:path";
import { act, useState } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import type {
  RuntimePlugin,
  RuntimePluginComponentDetail,
  RuntimePluginComponentState,
  RuntimePluginSetupItem,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { isPluginComponentUsableInChat, PluginConsole } from "./PluginConsole";

const skill: AwakenPluginConsoleComponent = {
  logicalId: "skill/example",
  logicalIdEncoded: "c2tpbGwvZXhhbXBsZQ",
  kind: "skill",
  name: "example-skill",
  description: "帮助用户完成示例任务",
  sourcePath: "skills/example/SKILL.md",
  sourceDialect: "claude_code",
  mappingState: "adapted",
  validation: "valid",
  desiredState: "enabled",
  configurationState: "ready",
  runtimeState: "failed",
  integrityState: "intact",
  switchable: true,
  diagnosticCount: 2,
};

function Harness() {
  const [detail, setDetail] = useState<AwakenPluginConsoleDetail | null>(null);
  const [activeTab, setActiveTab] = useState<"overview" | "preview" | "source" | "diagnostics">("preview");
  return (
    <AwakenPluginConsole
      name="example-plugin"
      displayName="Example Plugin"
      version="1.0.0"
      enabled
      components={[skill]}
      selectedDetail={detail}
      activeDetailTab={activeTab}
      onBack={() => undefined}
      onEnabledChange={() => undefined}
      onUninstall={() => undefined}
      onComponentOpen={(component) => {
        setActiveTab("preview");
        setDetail({
          component,
          preview: "# Skill preview",
          source: "# Skill source",
          diagnostics: [{ code: "sample_diagnostic", message: "示例诊断" }],
          resources: ["references/example.md"],
        });
      }}
      onComponentEnabledChange={() => undefined}
      onDetailTabChange={setActiveTab}
      onDetailClose={() => setDetail(null)}
      onUseInChat={() => undefined}
    />
  );
}

const routedComponents: AwakenPluginConsoleComponent[] = [
  ["mcp_server", "example-mcp"],
  ["rule", "example-rule"],
  ["agent", "example-agent"],
  ["command", "example-command"],
  ["hook", "example-hook"],
  ["variable", "example-variable"],
].map(([kind, name]) => ({
  ...skill,
  logicalId: `${kind}/${name}`,
  logicalIdEncoded: `${kind}-${name}`,
  kind,
  name,
  description: `${name} 的用途`,
  switchable: kind !== "variable",
	hook: kind === "hook" ? { sourceEvent: "PostToolUse", matcher: "Bash", condition: "Bash(git commit:*)", handlerType: "command" } : undefined,
}));

function DialogRoutingHarness() {
  const [detail, setDetail] = useState<AwakenPluginConsoleDetail | null>(null);
  const [activeTab, setActiveTab] = useState<"overview" | "preview" | "source" | "diagnostics">("overview");
  return (
    <AwakenPluginConsole
      name="routing-plugin"
      enabled
      components={routedComponents}
      selectedDetail={detail}
      detailFeedback={detail ? { message: "组件操作提示", tone: "info" } : null}
      activeDetailTab={activeTab}
      onBack={() => undefined}
      onEnabledChange={() => undefined}
      onUninstall={() => undefined}
      onComponentOpen={(component) => setDetail({ component, properties: {} })}
      onComponentEnabledChange={() => undefined}
      onDetailTabChange={setActiveTab}
      onDetailClose={() => setDetail(null)}
    />
  );
}

const mcpComponent: AwakenPluginConsoleComponent = {
  ...skill,
  logicalId: "mcp/example-server",
  logicalIdEncoded: "bWNwL2V4YW1wbGUtc2VydmVy",
  kind: "mcp_server",
  name: "example-server",
  mcp: {
    shortName: "example-server",
    transport: "stdio",
    connectionState: "ready",
    toolsLoaded: true,
    toolCount: 1,
  },
};

function McpHarness({ enabled = true }: { enabled?: boolean }) {
  const [detail, setDetail] = useState<AwakenPluginConsoleDetail | null>(null);
  return (
    <AwakenPluginConsole
      name="mcp-plugin"
      enabled={enabled}
      components={[mcpComponent]}
      selectedDetail={detail}
      activeDetailTab="overview"
      onBack={() => undefined}
      onEnabledChange={() => undefined}
      onUninstall={() => undefined}
      onComponentOpen={(component) => setDetail({
        component,
        mcp: {
          shortName: "example-server",
          environment: "local_process",
          connectionState: enabled ? "connected" : "stopped",
          toolsLoaded: true,
          tools: [{ name: "search", description: "搜索", enabled: true }],
        },
      })}
      onComponentEnabledChange={() => undefined}
      onDetailTabChange={() => undefined}
      onDetailClose={() => setDetail(null)}
      onMcpToolEnabledChange={() => undefined}
    />
  );
}

async function settle(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 0));
  });
}

function setTextAreaValue(element: HTMLTextAreaElement, value: string): void {
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLTextAreaElement.prototype,
    "value",
  )?.set;
  setter?.call(element, value);
  element.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("AwakenPluginConsole compact component flow", () => {
  it("uses the Outlook brand and removes uninstall from a built-in Plugin", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    await act(async () => {
      root.render(
        <AwakenPluginConsole
          name="awaken-outlook"
          displayName="Outlook 邮箱"
          builtin
          brandIcon="outlook"
          enabled={false}
          components={[]}
          selectedDetail={null}
          activeDetailTab="overview"
          onBack={() => undefined}
          onEnabledChange={() => undefined}
          onComponentOpen={() => undefined}
          onComponentEnabledChange={() => undefined}
          onDetailTabChange={() => undefined}
          onDetailClose={() => undefined}
        />,
      );
    });
    expect(container.textContent).toContain("Outlook 邮箱");
    expect(container.textContent).toContain("Awaken 内置");
    expect([...container.querySelectorAll("button")].some((button) => button.textContent === "卸载")).toBe(false);
    expect(container.querySelector('.aw-plugin-brand-icon[data-brand="outlook"][data-size="detail"] img')).not.toBeNull();
    await act(async () => root.unmount());
    container.remove();
  });

  it("keeps Plugin titles fully visible instead of ellipsizing them", () => {
    const styles = readStyleSource(resolve(
        process.cwd(),
        "../../Awaken Design System/src/components/AwakenPluginConsole/AwakenPluginConsole.css",
      ));
    const titleRule = styles.match(/\.aw-plugin-console__title h1 \{([^}]*)\}/)?.[1] ?? "";
    expect(titleRule).toContain("overflow-wrap: anywhere");
    expect(titleRule).toContain("line-height: 1.15");
    expect(titleRule).not.toContain("text-overflow: ellipsis");
    expect(titleRule).not.toContain("white-space: nowrap");
    expect(titleRule).not.toContain("overflow: hidden");
  });

  it("shows setup actions only in plugin details and keeps Apps inspect-only", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const onSetupAction = vi.fn();
    await act(async () => {
      root.render(
        <AwakenPluginConsole
          name="setup-plugin"
          enabled
          components={[skill]}
          setupItems={[
            { id: "account", kind: "oauth_pkce", title: "Outlook 账号", status: "needs_auth", message: "需要连接账号", actions: ["authorize"] },
            { id: "approval", kind: "oauth_pkce", title: "企业邮箱", status: "pending_admin_approval", message: "等待组织管理员批准", actions: ["retry"] },
            { id: "app", kind: "app_connector", title: "Codex App", status: "unsupported", connectorId: "connector_openai_mail", actions: ["inspect_only"] },
          ]}
          selectedDetail={null}
          activeDetailTab="overview"
          onBack={() => undefined}
          onEnabledChange={() => undefined}
          onUninstall={() => undefined}
          onComponentOpen={() => undefined}
          onComponentEnabledChange={() => undefined}
          onSetupAction={onSetupAction}
          onDetailTabChange={() => undefined}
          onDetailClose={() => undefined}
        />,
      );
    });
    expect(container.textContent).toContain("待完成设置");
    expect(container.textContent).toContain("Outlook 账号");
    expect(container.textContent).toContain("当前不支持");
    expect(container.textContent).toContain("等待管理员审批");
    expect(container.textContent).toContain("批准后重新连接");
    const connect = [...container.querySelectorAll<HTMLButtonElement>("button")].find((button) => button.textContent === "连接账号");
    expect(connect).not.toBeNull();
    expect(container.textContent).not.toContain("连接 Codex App");
    await act(async () => connect?.click());
    expect(onSetupAction).toHaveBeenCalledWith(expect.objectContaining({ id: "account" }), "authorize");
    await act(async () => root.unmount());
    container.remove();
  });

  it("shows an all-ready account as a separate connection row with account actions", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const onSetupAction = vi.fn();
    const readyItem: AwakenPluginSetupItem = {
      id: "outlook-account",
      kind: "host_connector",
      title: "连接 Outlook 账号",
      status: "ready",
      message: "宿主连接器账号已连接",
      actions: ["disconnect"],
    };
    await act(async () => {
      root.render(
        <AwakenPluginConsole
          name="awaken-outlook"
          displayName="Outlook 邮箱"
          brandIcon="outlook"
          enabled
          components={[skill]}
          setupItems={[readyItem]}
          selectedDetail={null}
          activeDetailTab="overview"
          onBack={() => undefined}
          onEnabledChange={() => undefined}
          onComponentOpen={() => undefined}
          onComponentEnabledChange={() => undefined}
          onSetupAction={onSetupAction}
          onDetailTabChange={() => undefined}
          onDetailClose={() => undefined}
        />,
      );
    });

    expect(container.textContent).toContain("Microsoft Outlook Email");
    expect(container.textContent).toContain("已连接");
    expect(container.textContent).not.toContain("待完成设置");
    expect(container.textContent).not.toContain("宿主连接器账号已连接");
    expect(container.querySelector('[role="switch"]')?.getAttribute("aria-label")).toBe("关闭Outlook 邮箱");
    const accountTrigger = container.querySelector<HTMLButtonElement>(".aw-plugin-account__trigger");
    expect(accountTrigger?.getAttribute("aria-label")).toContain("已连接");
    await act(async () => accountTrigger?.click());
    const reconnect = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')]
      .find((button) => button.textContent?.includes("重新连接"));
    await act(async () => reconnect?.click());
    expect(onSetupAction).toHaveBeenCalledWith(readyItem, "reauthorize");

    await act(async () => accountTrigger?.click());
    const disconnect = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')]
      .find((button) => button.textContent?.includes("断开连接"));
    await act(async () => disconnect?.click());
    expect(onSetupAction).toHaveBeenCalledWith(readyItem, "disconnect");

    await act(async () => root.unmount());
    container.remove();
  });

  it("keeps component keyboard focus on the rounded row instead of drawing an inner blue box", () => {
    const styles = readStyleSource(resolve(
        process.cwd(),
        "../../Awaken Design System/src/components/AwakenPluginConsole/AwakenPluginConsole.css",
      ));
    const keyboardSelector =
      ':root[data-aw-focus-modality="keyboard"] .aw-plugin-component-row:has(.aw-plugin-component-row__main:focus-visible) {';
    const keyboardRuleStart = styles.indexOf(keyboardSelector);
    const keyboardRuleEnd = styles.indexOf("}", keyboardRuleStart);
    const keyboardRule = styles.slice(keyboardRuleStart, keyboardRuleEnd + 1);

    expect(styles).toContain(
      ".aw-plugin-component-row__main:focus-visible { outline: none; box-shadow: none; }",
    );
    expect(keyboardRuleStart).toBeGreaterThanOrEqual(0);
    expect(keyboardRule).toContain("border-color: var(--aw-border-control-hover);");
    expect(keyboardRule).toContain("var(--aw-text-primary) 10%");
    expect(keyboardRule).not.toContain("var(--aw-shadow-focus)");
    expect(styles).toContain(
      ".aw-plugin-component-dialog .aw-dialog__head { align-items: center; padding-block: var(--aw-space-5); }",
    );
    expect(styles).toContain(
      ".aw-plugin-component-dialog .aw-dialog__title { display: flex; align-items: center; }",
    );
    expect(styles).toContain(
      ".aw-plugin-component-dialog .aw-dialog__head > span { align-self: center; }",
    );
    expect(styles).toContain(
      ".aw-plugin-component-dialog { width: min(780px, calc(100vw - 64px)); height: min(760px, calc(100vh - 48px)); max-height: min(760px, calc(100vh - 48px)); }",
    );
    expect(styles).toContain(
      ".aw-plugin-component-dialog { width: calc(100vw - 24px); height: calc(100vh - 24px); max-height: calc(100vh - 24px); }",
    );
    expect(styles).toContain(
      ".aw-plugin-component-dialog__source-footer { display: grid; width: 100%; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: var(--aw-space-3); }",
    );
    expect(styles).toContain(
      ".aw-plugin-component-dialog__feedback-footer { display: flex; width: 100%; align-items: center; justify-content: center; }",
    );
  });

  it("removes the local-source update action and keeps the plugin switch at the far right", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<Harness />);
    });

    const actions = container.querySelector<HTMLElement>(".aw-plugin-console__actions");
    const controls = [...(actions?.children ?? [])] as HTMLElement[];
    expect(actions?.textContent).not.toContain("检查更新");
    expect(controls).toHaveLength(2);
    expect(controls[0]?.textContent).toBe("卸载");
    expect(controls[1]?.getAttribute("role")).toBe("switch");
    expect(controls[1]?.getAttribute("aria-label")).toBe("关闭Example Plugin");

    await act(async () => {
      root.unmount();
    });
    container.remove();
  });

  it("opens Skill as a single Markdown preview without detail tabs", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<Harness />);
    });

    const pageText = container.textContent ?? "";
    expect(pageText).toContain("Example Plugin");
    expect(pageText).toContain("example-skill");
    expect(pageText).not.toContain("Claude Code");
    expect(pageText).not.toContain("claude_code");
    expect(pageText).not.toContain("skills/example/SKILL.md");
    expect(pageText).not.toContain("已适配");
    expect(pageText).not.toContain("运行失败");
    expect(pageText).not.toContain("完全访问提示");
    expect(document.querySelector('[role="dialog"]')).toBeNull();

    const componentButton = container.querySelector<HTMLButtonElement>(".aw-plugin-component-row__main");
    expect(componentButton).not.toBeNull();
    await act(async () => {
      componentButton?.click();
    });

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog).not.toBeNull();
    expect(dialog?.classList.contains("aw-plugin-component-dialog")).toBe(true);
    expect(document.querySelector(".aw-plugin-inspector")).toBeNull();
    expect(dialog?.textContent).toContain("Skill preview");
    expect(dialog?.textContent).toContain("在对话中使用");
    expect(dialog?.textContent).not.toContain("创建完整用户副本");
    expect(dialog?.textContent).not.toContain("Claude Code");
    expect(dialog?.querySelector(".aw-plugin-component-dialog__title small")).toBeNull();
    const useInChatButton = [...(dialog?.querySelectorAll<HTMLButtonElement>("button") ?? [])]
      .find((button) => button.textContent === "在对话中使用");
    expect(useInChatButton?.closest(".aw-dialog__foot")).not.toBeNull();
    expect(useInChatButton?.closest(".aw-dialog__body")).toBeNull();
    expect(dialog?.querySelector('[role="tablist"]')).toBeNull();
    expect(dialog?.textContent).not.toContain("详情");
    expect(dialog?.textContent).not.toContain("预览");
    expect(dialog?.textContent).not.toContain("源文件");
    expect(dialog?.textContent).not.toContain("Skill source");
    expect(dialog?.textContent).not.toContain("sample_diagnostic");
    expect(dialog?.textContent).not.toContain("示例诊断");

    const closeButton = dialog?.querySelector<HTMLButtonElement>('button[aria-label="关闭"]');
    expect(closeButton).not.toBeNull();
    await act(async () => {
      closeButton?.click();
    });
    expect(document.querySelector('[role="dialog"]')).toBeNull();

    await act(async () => {
      root.unmount();
    });
    container.remove();
  });

  it("routes the Skill footer action through the exact component selection", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const plugin: RuntimePlugin = {
      name: "skill-plugin",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/skill-plugin.zip" },
      digest: "sha256-v2:skill",
      skillCount: 1,
      serverCount: 0,
    };
    const state: RuntimePluginComponentState = {
      component: {
        logicalId: "skill/skills/review/SKILL.md",
        logicalIdEncoded: "encoded-skill",
        kind: "skill",
        displayName: "review",
        sourceDialect: "claude_code",
        sourcePath: "skills/review/SKILL.md",
        mappingState: "adapted",
        validation: "valid",
        skill: {
          path: "skills/review/SKILL.md",
          catalogId: "plugin__skill-plugin__review",
        },
      },
      desiredState: "enabled",
      configurationState: "ready",
      runtimeState: "ready",
      integrityState: "intact",
      mappingState: "adapted",
      switchable: true,
    };
    const runtimeClient = {
      getPluginSetup: vi.fn(async () => ({ items: [], summary: { total: 0, ready: 0, needsConfiguration: 0, needsAuth: 0, needsAdminApproval: 0, inProgress: 0, failed: 0, unsupported: 0 } })),

      listPluginComponents: vi.fn(async () => [state]),
      getPluginComponent: vi.fn(async () => ({
        state,
        preview: "# Review",
        sourceEditable: false,
        properties: {},
        resources: [],
      })),
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
    } as unknown as RuntimeClient;
    const onUsePlugin = vi.fn();
    const onUsePluginComponent = vi.fn();

    await act(async () => {
      root.render(
        <PluginConsole
          plugin={plugin}
          runtimeClient={runtimeClient}
          onBack={() => undefined}
          onPluginUpdated={() => undefined}
          onUninstall={() => undefined}
          onUsePlugin={onUsePlugin}
          onUsePluginComponent={onUsePluginComponent}
          onNotice={() => undefined}
        />,
      );
    });
    await settle();
    await act(async () => {
      container.querySelector<HTMLButtonElement>(".aw-plugin-component-row__main")?.click();
    });
    await settle();
    const useButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "在对话中使用");
    await act(async () => useButton?.click());

    expect(onUsePlugin).not.toHaveBeenCalled();
    expect(onUsePluginComponent).toHaveBeenCalledWith(plugin, state.component, []);

    await act(async () => root.unmount());
    container.remove();
  });

  it("reflects the parent Plugin switch immediately while Core persists the mutation", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const plugin: RuntimePlugin = {
      name: "awaken-outlook",
      displayName: "Outlook 邮箱",
      builtin: true,
      brandIcon: "outlook",
      enabled: false,
      source: { kind: "local_path", path: "/tmp/awaken-outlook" },
      digest: "sha256-v2:outlook-toggle",
      skillCount: 1,
      serverCount: 1,
    };
    const state: RuntimePluginComponentState = {
      component: {
        logicalId: "skill/skills/outlook/SKILL.md",
        logicalIdEncoded: "outlook-skill",
        kind: "skill",
        displayName: "Outlook 邮件",
        sourceDialect: "agent_plugins",
        sourcePath: "skills/outlook/SKILL.md",
        mappingState: "native",
        validation: "valid",
      },
      desiredState: "enabled",
      configurationState: "ready",
      runtimeState: "ready",
      integrityState: "intact",
      mappingState: "native",
      switchable: true,
    };
    let resolveEnable: ((value: RuntimePlugin) => void) | undefined;
    const pendingEnable = new Promise<RuntimePlugin>((resolvePromise) => {
      resolveEnable = resolvePromise;
    });
    const runtimeClient = {
      getPluginSetup: vi.fn(async () => ({ items: [], summary: { total: 0, ready: 0, needsConfiguration: 0, needsAuth: 0, needsAdminApproval: 0, inProgress: 0, failed: 0, unsupported: 0 } })),

      listPluginComponents: vi.fn(async () => [state]),
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
      enablePlugin: vi.fn(() => pendingEnable),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <PluginConsole
          plugin={plugin}
          runtimeClient={runtimeClient}
          onBack={() => undefined}
          onPluginUpdated={() => undefined}
          onUninstall={() => undefined}
          onUsePlugin={() => undefined}
          onUsePluginComponent={() => undefined}
          onNotice={() => undefined}
        />,
      );
    });
    await settle();
    const parentSwitch = container.querySelector<HTMLButtonElement>(
      '[role="switch"][aria-label="开启Outlook 邮箱"]',
    );
    await act(async () => {
      parentSwitch?.click();
      await Promise.resolve();
    });

    const optimisticSwitch = container.querySelector<HTMLButtonElement>(
      '[role="switch"][aria-label="关闭Outlook 邮箱"]',
    );
    expect(optimisticSwitch?.getAttribute("aria-checked")).toBe("true");
    expect(optimisticSwitch?.disabled).toBe(true);

    await act(async () => {
      resolveEnable?.({ ...plugin, enabled: true });
      await pendingEnable;
    });
    await settle();
    expect(runtimeClient.enablePlugin).toHaveBeenCalledWith(plugin.name);

    await act(async () => root.unmount());
    container.remove();
  });

  it("reopens a previously loaded plugin from its local snapshot without a skeleton", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const plugin: RuntimePlugin = {
      name: "cached-plugin",
      displayName: "Cached Plugin",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/cached-plugin" },
      digest: "sha256-v2:cached",
      skillCount: 1,
      serverCount: 0,
    };
    const state: RuntimePluginComponentState = {
      component: {
        logicalId: "skill/skills/cached/SKILL.md",
        logicalIdEncoded: "cached-skill",
        kind: "skill",
        displayName: "即时显示的技能",
        sourceDialect: "agent_plugins",
        sourcePath: "skills/cached/SKILL.md",
        mappingState: "native",
        validation: "valid",
      },
      desiredState: "enabled",
      configurationState: "ready",
      runtimeState: "ready",
      integrityState: "intact",
      mappingState: "native",
      switchable: true,
    };
    const never = new Promise<never>(() => undefined);
    const runtimeClient = {
      getPluginSetup: vi.fn(async () => ({ items: [], summary: { total: 0, ready: 0, needsConfiguration: 0, needsAuth: 0, needsAdminApproval: 0, inProgress: 0, failed: 0, unsupported: 0 } })),

      listPluginComponents: vi.fn()
        .mockResolvedValueOnce([state])
        .mockReturnValueOnce(never),
      getPluginConfiguration: vi.fn()
        .mockResolvedValueOnce({ fields: [], revision: 1 })
        .mockReturnValueOnce(never),
    } as unknown as RuntimeClient;
    const props = {
      plugin,
      runtimeClient,
      onBack: () => undefined,
      onPluginUpdated: () => undefined,
      onUninstall: () => undefined,
      onUsePlugin: () => undefined,
      onUsePluginComponent: () => undefined,
      onNotice: () => undefined,
    };

    const firstRoot = createRoot(container);
    await act(async () => firstRoot.render(<PluginConsole {...props} />));
    await settle();
    expect(container.textContent).toContain("即时显示的技能");
    await act(async () => firstRoot.unmount());

    const secondRoot = createRoot(container);
    await act(async () => secondRoot.render(<PluginConsole {...props} />));
    expect(container.textContent).toContain("即时显示的技能");
    expect(container.querySelector(".aw-plugin-console__loading")).toBeNull();

    await act(async () => secondRoot.unmount());
    container.remove();
  });

  it("keeps Core-owned authorization alive across Plugin Console remounts and resumes polling", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const plugin: RuntimePlugin = {
      name: "awaken-outlook",
      displayName: "Outlook 邮箱",
      brandIcon: "outlook",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/awaken-outlook" },
      digest: "sha256-v2:outlook-session",
      skillCount: 0,
      serverCount: 1,
    };
    const state: RuntimePluginComponentState = {
      component: {
        logicalId: "mcp/awaken-outlook",
        logicalIdEncoded: "outlook-mcp",
        kind: "mcp_server",
        displayName: "Outlook MCP",
        sourceDialect: "agent_plugins",
        sourcePath: ".awaken/outlook.json",
        mappingState: "native",
        validation: "valid",
        mcp: { shortName: "outlook", transport: "in-process", hostManaged: true },
      },
      desiredState: "enabled",
      configurationState: "ready",
      authenticationState: "needs_auth",
      runtimeState: "stopped",
      integrityState: "intact",
      mappingState: "native",
      switchable: false,
    };
    const authorizingSetup: RuntimePluginSetupItem = {
      requirement: {
        id: "outlook-account",
        fingerprint: "outlook-session-fingerprint",
        kind: "host_connector",
        title: "连接 Outlook 账号",
        connectorId: "awaken.microsoft.outlook",
        sourceDialect: "agent_plugins",
      },
      status: "authorizing",
      message: "正在连接 Outlook 账号。",
      actions: ["cancel", "mark_admin_approval"],
    };
    const setupResponse = {
      items: [authorizingSetup],
      summary: {
        total: 1,
        ready: 0,
        needsConfiguration: 0,
        needsAuth: 0,
        needsAdminApproval: 0,
        inProgress: 1,
        failed: 0,
        unsupported: 0,
      },
    };
    const pendingPolls: Array<Promise<never>> = [];
    const runtimeClient = {
      listPluginComponents: vi.fn(async () => [state]),
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
      getPluginSetup: vi.fn(async () => setupResponse),
      getPlugin: vi.fn(async () => plugin),
      getActivePluginAuthSession: vi.fn(async () => ({
        sessionId: "session-1",
        pluginName: plugin.name,
        setupId: authorizingSetup.requirement.id,
        status: "authorizing",
      })),
      getPluginAuthSession: vi.fn(() => {
        const pending = new Promise<never>(() => undefined);
        pendingPolls.push(pending);
        return pending;
      }),
      cancelPluginAuthSession: vi.fn(),
    } as unknown as RuntimeClient;
    const props = {
      plugin: { ...plugin, setupItems: [authorizingSetup] },
      runtimeClient,
      onBack: () => undefined,
      onPluginUpdated: () => undefined,
      onUninstall: () => undefined,
      onUsePlugin: () => undefined,
      onUsePluginComponent: () => undefined,
      onNotice: () => undefined,
    };
    const timeoutSpy = vi.spyOn(window, "setTimeout").mockImplementation((handler) => {
      if (typeof handler === "function") queueMicrotask(() => handler());
      return 1 as unknown as ReturnType<typeof setTimeout>;
    });

    try {
      const firstRoot = createRoot(container);
      await act(async () => firstRoot.render(<PluginConsole {...props} />));
      await settle();
      expect(runtimeClient.getActivePluginAuthSession).toHaveBeenCalledWith(
        plugin.name,
        authorizingSetup.requirement.id,
        {
          pluginDigest: plugin.digest,
          requirementFingerprint: authorizingSetup.requirement.fingerprint,
          force: false,
        },
      );
      expect(runtimeClient.getPluginAuthSession).toHaveBeenCalledTimes(1);

      await act(async () => firstRoot.unmount());
      expect(runtimeClient.cancelPluginAuthSession).not.toHaveBeenCalled();

      const secondRoot = createRoot(container);
      await act(async () => secondRoot.render(<PluginConsole {...props} />));
      await settle();
      expect(runtimeClient.getPluginAuthSession).toHaveBeenCalledTimes(2);
      expect(pendingPolls).toHaveLength(2);
      expect(runtimeClient.cancelPluginAuthSession).not.toHaveBeenCalled();

      await act(async () => secondRoot.unmount());
    } finally {
      timeoutSpy.mockRestore();
      container.remove();
    }
  });

  it("clears a terminal auth-session error when refreshed setup is already ready", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const plugin: RuntimePlugin = {
      name: "awaken-outlook",
      displayName: "Outlook 邮箱",
      brandIcon: "outlook",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/awaken-outlook" },
      digest: "sha256-v2:outlook",
      skillCount: 0,
      serverCount: 1,
    };
    const state: RuntimePluginComponentState = {
      component: {
        logicalId: "mcp/awaken-outlook",
        logicalIdEncoded: "outlook-mcp",
        kind: "mcp_server",
        displayName: "Outlook MCP",
        sourceDialect: "agent_plugins",
        sourcePath: ".awaken/outlook.json",
        mappingState: "native",
        validation: "valid",
        mcp: { shortName: "outlook", transport: "in-process", hostManaged: true },
      },
      desiredState: "enabled",
      configurationState: "ready",
      authenticationState: "ready",
      runtimeState: "connected",
      integrityState: "intact",
      mappingState: "native",
      switchable: false,
    };
    const readySetup: RuntimePluginSetupItem = {
      requirement: {
        id: "outlook-account",
        fingerprint: "outlook-fingerprint",
        kind: "host_connector",
        title: "连接 Outlook 账号",
        connectorId: "awaken.microsoft.outlook",
        sourceDialect: "agent_plugins",
      },
      status: "ready",
      message: "Outlook 账号与工具运行时已就绪。",
      actions: ["disconnect"],
    };
    const setupResponse = {
      items: [readySetup],
      summary: {
        total: 1,
        ready: 1,
        needsConfiguration: 0,
        needsAuth: 0,
        needsAdminApproval: 0,
        inProgress: 0,
        failed: 0,
        unsupported: 0,
      },
    };
    const runtimeClient = {
      listPluginComponents: vi.fn(async () => [state]),
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
      getPluginSetup: vi.fn(async () => setupResponse),
      getPlugin: vi.fn(async () => plugin),
      startPluginSetup: vi.fn(async () => ({ sessionId: "session-1", status: "authorizing" })),
      getPluginAuthSession: vi.fn(async () => ({
        sessionId: "session-1",
        pluginName: plugin.name,
        setupId: readySetup.requirement.id,
        status: "failed",
        errorMessage: "账号已授权，但 Outlook 工具运行时未就绪。",
      })),
    } as unknown as RuntimeClient;
    const onNotice = vi.fn();
    const timeoutSpy = vi.spyOn(window, "setTimeout").mockImplementation((handler) => {
      if (typeof handler === "function") queueMicrotask(() => handler());
      return 1 as unknown as ReturnType<typeof setTimeout>;
    });

    try {
      await act(async () => {
        root.render(
          <PluginConsole
            plugin={{ ...plugin, setupItems: [readySetup] }}
            runtimeClient={runtimeClient}
            onBack={() => undefined}
            onPluginUpdated={() => undefined}
            onUninstall={() => undefined}
            onUsePlugin={() => undefined}
            onUsePluginComponent={() => undefined}
            onNotice={onNotice}
          />,
        );
      });
      await settle();
      const accountTrigger = container.querySelector<HTMLButtonElement>(".aw-plugin-account__trigger");
      await act(async () => accountTrigger?.click());
      const reconnect = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')]
        .find((button) => button.textContent?.includes("重新连接"));
      await act(async () => reconnect?.click());
      await settle();
      await settle();

      expect(runtimeClient.getPluginAuthSession).toHaveBeenCalledWith(plugin.name, "session-1");
      expect(container.querySelector(".aw-plugin-console__error")).toBeNull();
      expect(container.textContent).toContain("已连接");
      expect(onNotice).toHaveBeenCalledWith("插件账号已连接，关联组件状态已刷新");
    } finally {
      timeoutSpy.mockRestore();
      await act(async () => root.unmount());
      container.remove();
    }
  });

  it("uses the same independent dialog for MCP, rules, agents, commands, hooks and variables", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    await act(async () => {
      root.render(<DialogRoutingHarness />);
    });
	expect(
		[...container.querySelectorAll<HTMLButtonElement>(".aw-plugin-component-row__main")]
			.find((item) => item.textContent?.includes("example-hook"))?.textContent,
	).toContain("Bash(git commit:*)");

    for (const component of routedComponents) {
      const button = [...container.querySelectorAll<HTMLButtonElement>(".aw-plugin-component-row__main")]
        .find((item) => item.textContent?.includes(component.name));
      expect(button).not.toBeNull();
      await act(async () => {
        button?.click();
      });
      const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
      expect(dialog?.textContent).toContain(component.name);
      expect(dialog?.querySelector(".aw-plugin-component-dialog__title small")).toBeNull();
      expect(dialog?.querySelector('[role="tablist"]')).not.toBeNull();
      expect(document.querySelector(".aw-plugin-inspector")).toBeNull();
      const feedback = dialog?.querySelector<HTMLElement>(".aw-plugin-component-dialog__feedback");
      expect(feedback?.textContent).toBe("组件操作提示");
      expect(feedback?.closest(".aw-dialog__foot")).not.toBeNull();
      expect(dialog?.querySelector(".aw-plugin-console__error")).toBeNull();
      const closeButton = dialog?.querySelector<HTMLButtonElement>('button[aria-label="关闭"]');
      await act(async () => {
        closeButton?.click();
      });
      expect(document.querySelector('[role="dialog"]')).toBeNull();
    }

    await act(async () => {
      root.unmount();
    });
    container.remove();
  });

  it("labels recognized Apps as inventory-only instead of presenting a runnable action", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const appComponent: AwakenPluginConsoleComponent = {
      ...skill,
      logicalId: "app/manifest.json",
      logicalIdEncoded: "app-encoded",
      kind: "app",
      name: "plugin-management",
      mappingState: "recognized_unsupported",
      desiredState: "disabled",
      runtimeState: "unavailable",
      switchable: false,
    };
    await act(async () => {
      root.render(
        <AwakenPluginConsole
          name="app-plugin"
          enabled
          components={[appComponent]}
          selectedDetail={{ component: appComponent, properties: {} }}
          activeDetailTab="overview"
          onBack={() => undefined}
          onEnabledChange={() => undefined}
          onUninstall={() => undefined}
          onComponentOpen={() => undefined}
          onComponentEnabledChange={() => undefined}
          onDetailTabChange={() => undefined}
          onDetailClose={() => undefined}
          onUseInChat={() => undefined}
        />,
      );
    });

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.textContent).toContain("当前仅识别该组件");
    expect(dialog?.textContent).toContain("不会把它作为可调用能力交给模型");
    expect(dialog?.textContent).not.toContain("在对话中使用");
    expect(dialog?.textContent).not.toContain("插入 Composer");

    await act(async () => root.unmount());
    container.remove();
  });

  it("removes the redundant manual MCP reload action", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    await act(async () => {
      root.render(<McpHarness />);
    });
    await act(async () => {
      container.querySelector<HTMLButtonElement>(".aw-plugin-component-row__main")?.click();
    });

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.textContent).toContain("MCP 运行状态");
    expect(dialog?.textContent).not.toContain("重新加载");

    await act(async () => root.unmount());
    container.remove();
  });

  it("shows a host-managed Outlook MCP in the normal MCP group with read-only runtime controls", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const outlookMcp: AwakenPluginConsoleComponent = {
      ...mcpComponent,
      logicalId: "mcp_server/.awaken/setup.json#host_connector:outlook-account",
      logicalIdEncoded: "host-outlook-mcp",
      name: "Outlook MCP",
      switchable: false,
      authenticationState: "ready",
      mcp: {
        shortName: "awaken-outlook",
        transport: "in-process",
        connectionState: "connected",
        toolsLoaded: true,
        toolCount: 1,
        hostManaged: true,
        connectorId: "awaken.microsoft.outlook",
      },
    };
    await act(async () => {
      root.render(
        <AwakenPluginConsole
          name="awaken-outlook"
          enabled
          components={[outlookMcp]}
          selectedDetail={{
            component: outlookMcp,
            properties: {},
            mcp: {
              shortName: "awaken-outlook",
              environment: "local_process",
              connectionState: "connected",
              toolsLoaded: true,
              tools: [{ name: "list_messages", description: "读取邮件列表", enabled: true }],
              hostManaged: true,
              connectorId: "awaken.microsoft.outlook",
            },
          }}
          activeDetailTab="overview"
          onBack={() => undefined}
          onEnabledChange={() => undefined}
          onUninstall={() => undefined}
          onComponentOpen={() => undefined}
          onComponentEnabledChange={() => undefined}
          onDetailTabChange={() => undefined}
          onDetailClose={() => undefined}
          onMcpToolEnabledChange={() => undefined}
        />,
      );
    });

    expect(container.textContent).toContain("MCP 服务");
    expect(container.textContent).toContain("Outlook MCP");
    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.textContent).toContain("由 Awaken 连接器管理");
    expect(dialog?.textContent).toContain("list_messages");
	expect(dialog?.textContent).not.toContain("hostManaged");
    expect(dialog?.querySelector<HTMLButtonElement>('[role="switch"][aria-label*="list_messages"]')?.disabled).toBe(true);

    await act(async () => root.unmount());
    container.remove();
  });

  it("shows Command arguments only when Core says the template accepts them", async () => {
	const container = document.createElement("div");
	document.body.appendChild(container);
	const root = createRoot(container);
	const commandComponent: AwakenPluginConsoleComponent = {
		...skill,
		logicalId: "command/review",
		logicalIdEncoded: "command-review",
		kind: "command",
		name: "review",
	};
	const renderCommand = async (acceptsArguments: boolean, argumentsRequired = false) => {
		await act(async () => {
			root.render(
				<AwakenPluginConsole
					name="command-plugin"
					enabled
					components={[commandComponent]}
					selectedDetail={{
						component: commandComponent,
						properties: {},
						command: { acceptsArguments, argumentsRequired, argumentHint: "<file>" },
					}}
					activeDetailTab="overview"
					onBack={() => undefined}
					onEnabledChange={() => undefined}
					onUninstall={() => undefined}
					onComponentOpen={() => undefined}
					onComponentEnabledChange={() => undefined}
					onDetailTabChange={() => undefined}
					onDetailClose={() => undefined}
					onUseInChat={() => undefined}
				/>,
			);
		});
	};

	await renderCommand(false);
	expect(document.querySelector('input[placeholder="<file>"]')).toBeNull();
	await renderCommand(true, true);
	expect(document.querySelector('input[placeholder="<file>"]')).not.toBeNull();
	expect([...document.querySelectorAll<HTMLButtonElement>("button")].find((button) => button.textContent?.includes("插入 Composer"))?.disabled).toBe(true);

	await act(async () => root.unmount());
	container.remove();
  });

  it("keeps invalid Command components inspectable but removes the Composer action", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const invalidCommand: AwakenPluginConsoleComponent = {
      ...skill,
      logicalId: "command/invalid",
      logicalIdEncoded: "command-invalid",
      kind: "command",
      name: "invalid-command",
      mappingState: "invalid",
      validation: "invalid",
      desiredState: "enabled",
      switchable: false,
      command: { invocationName: "/plugin:invalid-command" },
    };

    await act(async () => {
      root.render(
        <AwakenPluginConsole
          name="command-plugin"
          enabled
          components={[invalidCommand]}
          selectedDetail={{
            component: invalidCommand,
            properties: {},
            diagnostics: [{ code: "plugin_command_invalid", message: "缺少 description" }],
            command: { acceptsArguments: false, argumentsRequired: false },
          }}
          activeDetailTab="overview"
          onBack={() => undefined}
          onEnabledChange={() => undefined}
          onUninstall={() => undefined}
          onComponentOpen={() => undefined}
          onComponentEnabledChange={() => undefined}
          onDetailTabChange={() => undefined}
          onDetailClose={() => undefined}
          onUseInChat={() => undefined}
        />,
      );
    });

    const dialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(dialog?.textContent).toContain("invalid-command");
    expect(dialog?.textContent).not.toContain("插入 Composer");

    await act(async () => root.unmount());
    container.remove();
  });

  it("rejects stale or invalid component state before creating a Composer token", () => {
    const invalidState: RuntimePluginComponentState = {
      component: {
        logicalId: "command/invalid",
        logicalIdEncoded: "command-invalid",
        kind: "command",
        displayName: "invalid-command",
        sourceDialect: "codex",
        sourcePath: "commands/invalid.md",
        mappingState: "invalid",
        validation: "invalid",
      },
      desiredState: "enabled",
      configurationState: "ready",
      runtimeState: "not_applicable",
      integrityState: "intact",
      mappingState: "invalid",
      switchable: false,
    };
    expect(isPluginComponentUsableInChat(true, invalidState)).toBe(false);
    expect(isPluginComponentUsableInChat(false, {
      ...invalidState,
      component: { ...invalidState.component, mappingState: "adapted", validation: "valid" },
      mappingState: "adapted",
      switchable: true,
    })).toBe(false);
  });

  it("shows and locks child component controls off while the plugin is disabled", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    await act(async () => {
      root.render(<McpHarness enabled={false} />);
    });

    const componentSwitch = container.querySelector<HTMLButtonElement>(
      '.aw-plugin-component-row [role="switch"]',
    );
    expect(componentSwitch?.getAttribute("aria-checked")).toBe("false");
    expect(componentSwitch?.disabled).toBe(true);

    await act(async () => {
      container.querySelector<HTMLButtonElement>(".aw-plugin-component-row__main")?.click();
    });
    const toolSwitch = document.querySelector<HTMLButtonElement>(
      '.aw-plugin-component-dialog [role="switch"]',
    );
    expect(toolSwitch?.getAttribute("aria-checked")).toBe("false");
    expect(toolSwitch?.disabled).toBe(true);

    await act(async () => root.unmount());
    container.remove();
  });

  it("shows raw MCP source and saves edits through the product runtime client", async () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    const plugin: RuntimePlugin = {
      name: "mcp-source-plugin",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/mcp-source-plugin.zip" },
      digest: "sha256-v2:source-v1",
      skillCount: 0,
      serverCount: 1,
    };
    const state: RuntimePluginComponentState = {
      component: {
        logicalId: "mcp/mcp.json#local",
        logicalIdEncoded: "bWNwL21jcC5qc29uI2xvY2Fs",
        revisionId: "revision-v1",
        kind: "mcp_server",
        displayName: "local",
        sourceDialect: "agent_plugins",
        sourcePath: "mcp.json#local",
        mappingState: "native",
        validation: "valid",
        mcp: { shortName: "local", transport: "stdio" },
      },
      desiredState: "enabled",
      configurationState: "ready",
      runtimeState: "connected",
      integrityState: "intact",
      mappingState: "native",
      switchable: true,
    };
    const originalSource = `{"mcpServers":{"local":{"command":"npx","args":["--token","literal-value"]}}}`;
    const editedSource = `{"mcpServers":{"local":{"command":"node","args":["server.js"]}}}`;
    let currentSource = originalSource;
    const detail = (): RuntimePluginComponentDetail => ({
      state,
      source: currentSource,
      sourceEditable: true,
      properties: {},
      resources: [],
    });
    const updatedPlugin = { ...plugin, digest: "sha256-v2:source-v2" };
    const updatePluginComponentSource = vi.fn(
      async (
        _name: string,
        _kind: string,
        _logicalId: string,
        request: { expectedDigest: string; source: string },
      ) => {
        if (request.source === "{not-json") {
          throw new Error("MCP_SOURCE_INVALID: mcp.json is not valid JSON");
        }
        const changed = request.source !== currentSource;
        if (changed) currentSource = request.source;
        return {
          plugin: changed ? updatedPlugin : plugin,
          revision: changed ? 2 : 1,
          changed,
        };
      },
    );
    const runtimeClient = {
      getPluginSetup: vi.fn(async () => ({ items: [], summary: { total: 0, ready: 0, needsConfiguration: 0, needsAuth: 0, needsAdminApproval: 0, inProgress: 0, failed: 0, unsupported: 0 } })),

      listPluginComponents: vi.fn(async () => [state]),
      getPluginComponent: vi.fn(async () => detail()),
      updatePluginComponentSource,
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
    } as unknown as RuntimeClient;
    const onPluginUpdated = vi.fn();
    const onNotice = vi.fn();

    await act(async () => {
      root.render(
        <PluginConsole
          plugin={plugin}
          runtimeClient={runtimeClient}
          onBack={() => undefined}
          onPluginUpdated={onPluginUpdated}
          onUninstall={() => undefined}
          onUsePlugin={() => undefined}
          onUsePluginComponent={() => undefined}
          onNotice={onNotice}
        />,
      );
    });
    await settle();

    await act(async () => {
      container.querySelector<HTMLButtonElement>(".aw-plugin-component-row__main")?.click();
    });
    await settle();
    const sourceTab = [...document.querySelectorAll<HTMLButtonElement>('[role="tab"]')]
      .find((button) => button.textContent === "源文件");
    await act(async () => sourceTab?.click());
    const sourceSurface = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="MCP 源文件"]');
    expect(sourceSurface?.value).toBe(originalSource);
    expect(sourceSurface?.readOnly).toBe(true);
    expect(sourceSurface?.dataset.editing).toBe("false");
    expect(sourceSurface?.value).toContain("literal-value");
    expect(sourceSurface?.value).not.toContain("<configured>");

    const editButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "编辑源文件");
    expect(editButton?.closest(".aw-dialog__foot")).not.toBeNull();
    expect(editButton?.style.background).toBe("var(--aw-bg-fill-light)");
    expect(editButton?.style.transition).toBe("none");
    if (sourceSurface) sourceSurface.scrollTop = 72;
    await act(async () => editButton?.click());
    const editor = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="MCP 源文件"]');
    const cancelButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "取消");
    const initialSaveButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "保存");
    expect(cancelButton).not.toBe(editButton);
    expect(initialSaveButton).not.toBe(editButton);
    expect(cancelButton?.style.background).toBe("var(--aw-bg-fill-light)");
    expect(cancelButton?.style.transition).toBe("none");
    expect(initialSaveButton?.style.transition).toBe("none");
    expect(initialSaveButton?.disabled).toBe(false);
    expect(editor).toBe(sourceSurface);
    expect(editor?.value).toBe(originalSource);
    expect(editor?.readOnly).toBe(false);
    expect(editor?.dataset.editing).toBe("true");
    expect(editor?.scrollTop).toBe(72);

    await act(async () => {
      if (editor) setTextAreaValue(editor, "{not-json");
    });
    await act(async () => initialSaveButton?.click());
    await settle();
    const errorFeedback = document.querySelector<HTMLElement>(".aw-plugin-component-dialog__feedback");
    expect(errorFeedback?.textContent).toContain("MCP 源文件格式不符合要求");
    expect(errorFeedback?.dataset.tone).toBe("error");
    expect(errorFeedback?.getAttribute("role")).toBe("alert");
    expect(errorFeedback?.closest(".aw-dialog__foot")).not.toBeNull();
    expect(errorFeedback?.nextElementSibling?.contains(initialSaveButton ?? null)).toBe(true);
    expect(document.querySelector(".aw-plugin-component-dialog .aw-plugin-console__error")).toBeNull();
    expect(editor?.readOnly).toBe(false);

    await act(async () => {
      if (editor) setTextAreaValue(editor, originalSource);
    });
    expect(document.body.textContent).not.toContain("MCP 源文件格式不符合要求");
    const unchangedSaveButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "保存");
    expect(unchangedSaveButton?.disabled).toBe(false);
    await act(async () => unchangedSaveButton?.click());
    await settle();

    expect(updatePluginComponentSource).toHaveBeenNthCalledWith(
      2,
      "mcp-source-plugin",
      "mcp_server",
      state.component.logicalIdEncoded,
      { expectedDigest: "sha256-v2:source-v1", source: originalSource },
    );
    expect(onPluginUpdated).toHaveBeenCalledWith(plugin);
    const unchangedFeedback = document.querySelector<HTMLElement>(".aw-plugin-component-dialog__feedback");
    expect(unchangedFeedback?.textContent).toBe("MCP 源文件内容未变化");
    expect(unchangedFeedback?.dataset.tone).toBe("info");
    expect(unchangedFeedback?.closest(".aw-dialog__foot")).not.toBeNull();
    expect(editor?.readOnly).toBe(true);
    expect(editor?.scrollTop).toBe(72);

    const editAgainButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "编辑源文件");
    await act(async () => editAgainButton?.click());
    expect(document.querySelector(".aw-plugin-component-dialog__feedback")).toBeNull();
    await act(async () => {
      if (editor) setTextAreaValue(editor, editedSource);
    });
    const saveButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "保存");
    await act(async () => saveButton?.click());
    await settle();

    expect(updatePluginComponentSource).toHaveBeenNthCalledWith(
      3,
      "mcp-source-plugin",
      "mcp_server",
      state.component.logicalIdEncoded,
      { expectedDigest: "sha256-v2:source-v1", source: editedSource },
    );
    expect(onPluginUpdated).toHaveBeenLastCalledWith(updatedPlugin);
    const savedFeedback = document.querySelector<HTMLElement>(".aw-plugin-component-dialog__feedback");
    expect(savedFeedback?.textContent).toBe("MCP 源文件已保存，插件组件与运行状态已刷新");
    expect(savedFeedback?.dataset.tone).toBe("success");
    expect(savedFeedback?.closest(".aw-dialog__foot")).not.toBeNull();
    expect(onNotice).not.toHaveBeenCalled();
    const savedSurface = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="MCP 源文件"]');
    expect(savedSurface).toBe(sourceSurface);
    expect(savedSurface?.value).toBe(editedSource);
    expect(savedSurface?.readOnly).toBe(true);
    expect(savedSurface?.dataset.editing).toBe("false");
    expect(savedSurface?.scrollTop).toBe(72);

    await act(async () => root.unmount());
    container.remove();
  });
});
