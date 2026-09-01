// @vitest-environment jsdom

import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  RuntimePlugin,
  RuntimePluginComponentDetail,
  RuntimePluginComponentState,
  RuntimePluginSource,
  RuntimeProject,
  RuntimeSkill,
} from "../shared/runtime";
import {
  ExtensionsView,
  SKILL_CATALOG_SYNC_INTERVAL_MS,
  type ExtensionSection,
} from "./ExtensionsView";
import type { RuntimeClient } from "./runtime/client";

const projectRoot = "/workspace/marketing";

const projectSkill: RuntimeSkill = {
  id: "project-report",
  catalogEntryId: "catalog-project-report",
  name: "project-report",
  displayName: "项目报告",
  description: "生成项目报告",
  state: "loaded",
  sourceOrigin: "project",
  rootDir: `${projectRoot}/.awaken/skills/project-report`,
  skillFilePath: `${projectRoot}/.awaken/skills/project-report/SKILL.md`,
  projectPath: projectRoot,
  context: "inline",
};

const runtimeProject: RuntimeProject = {
  projectId: "project-marketing",
  workspaceRoot: projectRoot,
  folders: [{ path: projectRoot, role: "user", isPrimary: true }],
  displayName: "营销项目",
  state: "active",
  createdAt: "2026-07-27T00:00:00Z",
  updatedAt: "2026-07-27T00:00:00Z",
  version: 1,
};

function RoutedInstallHarness({
  initialSection,
  runtimeClient,
  onNotice,
}: {
  initialSection: "skills" | "plugins";
  runtimeClient: RuntimeClient;
  onNotice: (message: string) => void;
}) {
  const [section, setSection] = useState<ExtensionSection>(initialSection);
  return (
    <ExtensionsView
      section={section}
      desktopReady
      runtimeClient={runtimeClient}
      activeWorkspaceRoot={projectRoot}
      onNotice={onNotice}
      onNavigateSection={setSection}
    />
  );
}

const bundledSkill: RuntimeSkill = {
  ...projectSkill,
  id: "bundled-report",
  catalogEntryId: "catalog-bundled-report",
  name: "bundled-report",
  displayName: "内置报告",
  sourceOrigin: "bundled",
  rootDir: "/runtime/system-skills/bundled-report",
  skillFilePath: "/runtime/system-skills/bundled-report/SKILL.md",
};

const globalAgentsSkill: RuntimeSkill = {
  ...projectSkill,
  id: "agents-global-dws",
  catalogEntryId: "catalog-agents-global-dws",
  name: "dws",
  displayName: "DWS",
  description: "管理钉钉产品能力",
  sourceOrigin: "agents",
  rootDir: "/Users/test/.agents/skills/dws",
  skillFilePath: "/Users/test/.agents/skills/dws/SKILL.md",
};

const projectAgentsSkill: RuntimeSkill = {
  ...projectSkill,
  id: "agents-project-review",
  catalogEntryId: "catalog-agents-project-review",
  name: "project-review",
  displayName: "项目代码复核",
  description: "按项目规范执行变更复核",
  sourceOrigin: "agents",
  rootDir: `${projectRoot}/.agents/skills/project-review`,
  skillFilePath: `${projectRoot}/.agents/skills/project-review/SKILL.md`,
};

const pluginSkill: RuntimeSkill = {
  ...projectSkill,
  id: "plugin__console-plugin__private-review",
  catalogEntryId: "catalog-plugin-private-review",
  name: "private-review",
  displayName: "插件私有复核",
  description: "只在 Plugin Console 中管理的插件 Skill",
  sourceOrigin: "plugin",
  rootDir: "/Users/test/.awaken/plugins/packages/console.plugin/skills/private-review",
  skillFilePath:
    "/Users/test/.awaken/plugins/packages/console.plugin/skills/private-review/SKILL.md",
  projectPath: undefined,
};

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function buttonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.trim() === text,
  );
}

async function setInputValue(
  input: HTMLInputElement | null,
  value: string,
): Promise<void> {
  await act(async () => {
    if (!input) return;
    const setter = Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )?.set;
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await settle();
}

function assertSplitPreviewCards(root: ParentNode): void {
  const content = root.querySelector(".aw-skill-preview__content");
  const resourceCard = root.querySelector(".aw-skill-preview__card--resources");
  const documentCard = root.querySelector(".aw-skill-preview__card--document");
  expect(content).not.toBeNull();
  expect(resourceCard).not.toBeNull();
  expect(documentCard).not.toBeNull();
  expect(resourceCard?.parentElement).toBe(content);
  expect(documentCard?.parentElement).toBe(content);
  expect(resourceCard?.nextElementSibling).toBe(documentCard);
  expect(documentCard?.querySelector(".aw-skill-preview__card--resources")).toBeNull();
  expect(documentCard?.textContent).toContain("SKILL.md");
}

describe("ExtensionsView behavior", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: vi.fn(),
    });
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("clears visible loading after a silent refresh supersedes its data generation", async () => {
    let resolveInitialRuntimeInfo: ((value: { defaultWorkspace: string }) => void) | undefined;
    const initialRuntimeInfo = new Promise<{ defaultWorkspace: string }>((resolve) => {
      resolveInitialRuntimeInfo = resolve;
    });
    const getRuntimeInfo = vi
      .fn<() => Promise<{ defaultWorkspace: string }>>()
      .mockImplementationOnce(() => initialRuntimeInfo)
      .mockResolvedValue({ defaultWorkspace: projectRoot });
    const runtimeClient = {
      getRuntimeInfo,
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="connectors"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    const refreshButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="刷新"]',
    );
    expect(getRuntimeInfo).toHaveBeenCalledTimes(2);
    expect(refreshButton?.disabled).toBe(true);

    await act(async () => {
      resolveInitialRuntimeInfo?.({ defaultWorkspace: projectRoot });
      await initialRuntimeInfo;
    });
    await settle();

    expect(refreshButton?.disabled).toBe(false);
  });

  it("connects an OAuth host connector through a browser session without exposing MCP server settings", async () => {
    let connected = false;
    const onNotice = vi.fn();
    const listConnectors = vi.fn(async () => [{
      id: "host.mail.oauth",
      kind: "mcp" as const,
      displayName: "企业邮箱",
      description: "企业邮箱连接器",
      category: "chat",
      provider: "Mail Provider",
      state: connected ? "connected" as const : "needs-auth" as const,
      sourceOrigin: "builtin",
      authType: "oauth2",
      serverNames: ["host-mail"],
    }]);
    const connectConnector = vi.fn(async () => ({
      connectorId: "host.mail.oauth",
      connectorState: "authorizing",
      sessionId: "connector-session",
      authStatus: "authorizing" as const,
      message: "请在浏览器完成企业邮箱授权",
    }));
    const getConnectorAuthSession = vi.fn(async () => {
      connected = true;
      return {
        sessionId: "connector-session",
        connectorId: "host.mail.oauth",
        status: "ready" as const,
      };
    });
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors,
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => [{
        name: "host-mail",
        displayName: "企业邮箱",
        state: connected ? "connected" as const : "needs-auth" as const,
        transport: "in-process",
        scope: "user",
        sourceConnectorId: "host.mail.oauth",
      }]),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      connectConnector,
      disconnectConnector: vi.fn(),
      getConnectorAuthSession,
      cancelConnectorAuthSession: vi.fn(),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="connectors"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={onNotice}
        />,
      );
    });
    await settle();

    expect(document.querySelector('button[aria-label="设置 企业邮箱"]')).toBeNull();
    const connectButton = document.querySelector<HTMLButtonElement>('button[aria-label="连接 企业邮箱"]');
    expect(connectButton).not.toBeNull();
    await act(async () => connectButton?.click());
    await act(async () => new Promise((resolve) => setTimeout(resolve, 850)));

    expect(connectConnector).toHaveBeenCalledWith("host.mail.oauth");
    expect(getConnectorAuthSession).toHaveBeenCalledWith("connector-session");
    expect(onNotice).toHaveBeenCalledWith("企业邮箱 已连接");
    expect(document.querySelector('button[aria-label="设置 企业邮箱"]')).toBeNull();
    expect(document.querySelector('button[aria-label="断开 企业邮箱"]')).not.toBeNull();
  });

  it("records an external administrator approval page instead of leaving OAuth spinning", async () => {
    let approvalPending = false;
    const onNotice = vi.fn();
    const listConnectors = vi.fn(async () => [{
      id: "host.mail.oauth",
      kind: "mcp" as const,
      displayName: "企业邮箱",
      description: "企业邮箱连接器",
      category: "chat",
      provider: "Mail Provider",
      state: approvalPending ? "approval-required" as const : "needs-auth" as const,
      lastError: approvalPending ? "等待组织管理员批准；批准后请重新连接。" : undefined,
      sourceOrigin: "builtin",
      authType: "oauth2",
    }]);
    const markConnectorAdminApprovalPending = vi.fn(async () => {
      approvalPending = true;
      return {
        sessionId: "approval-session",
        connectorId: "host.mail.oauth",
        status: "pending_admin_approval" as const,
      };
    });
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors,
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      connectConnector: vi.fn(async () => ({
        connectorId: "host.mail.oauth",
        connectorState: "authorizing",
        sessionId: "approval-session",
        authStatus: "authorizing" as const,
      })),
      disconnectConnector: vi.fn(),
      getConnectorAuthSession: vi.fn(async () => ({
        sessionId: "approval-session",
        connectorId: "host.mail.oauth",
        status: approvalPending ? "pending_admin_approval" as const : "authorizing" as const,
      })),
      cancelConnectorAuthSession: vi.fn(),
      markConnectorAdminApprovalPending,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="connectors"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={onNotice}
        />,
      );
    });
    await settle();
    await act(async () => document.querySelector<HTMLButtonElement>('button[aria-label="连接 企业邮箱"]')?.click());
    await settle();
    const approvalButton = [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((button) => button.textContent === "页面要求管理员审批");
    expect(approvalButton).not.toBeNull();
    await act(async () => approvalButton?.click());
    await settle();

    expect(markConnectorAdminApprovalPending).toHaveBeenCalledWith("approval-session");
    expect(document.body.textContent).toContain("等待管理员审批");
    expect(onNotice).toHaveBeenCalledWith("企业邮箱 已记录为等待组织管理员审批");
  });

  it("keeps plugin-only host connectors out of the Connector catalog", async () => {
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => [{
        id: "awaken.microsoft.outlook",
        kind: "mcp" as const,
        displayName: "Outlook",
        description: "Outlook 邮件",
        category: "chat",
        provider: "Microsoft",
        state: "needs-auth" as const,
        sourceOrigin: "builtin",
        authType: "oauth2",
        catalogPlacement: "plugin_only" as const,
        serverNames: ["awaken-outlook"],
      }]),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => [{
        name: "awaken-outlook",
        displayName: "Outlook",
        state: "needs-auth" as const,
        transport: "in-process",
        scope: "user",
        sourceConnectorId: "awaken.microsoft.outlook",
      }]),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="connectors"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    expect(document.body.textContent).not.toContain("Outlook 邮件");
    expect(document.querySelector('[aria-label="连接 Outlook"]')).toBeNull();
    expect(document.body.textContent).not.toContain("awaken-outlook");
  });

  it("automatically validates a selected ZIP and replaces a same-name Plugin", async () => {
    const plugin: RuntimePlugin = {
      name: "acme.tools",
      version: "1.2.0",
      description: "团队代码检查工具",
      dialect: "cursor",
      manifestPath: ".cursor-plugin/plugin.json",
      compatibilityState: "partial",
      componentTotals: { total: 23, runnable: 13, needsConfiguration: 0, unsupported: 1, ignored: 5, invalid: 4 },
      componentCounts: { skill: 12, mcp: 1, rule: 5, agent: 2, command: 1, hook: 2 },
      enabled: true,
      source: {
        kind: "local_path",
        path: "/tmp/acme.tools.zip",
      },
      skillCount: 2,
      serverCount: 1,
    };
    const pickPluginSource = vi
      .fn()
      .mockResolvedValueOnce({
        canceled: false as const,
        path: "/tmp/acme-plugin.zip",
      })
      .mockResolvedValueOnce({
        canceled: false as const,
        path: "/tmp/acme-plugin-v2.zip",
      });
    const validatePlugin = vi.fn(async (source: RuntimePluginSource) => ({
      valid: true,
      source,
      name: "acme.tools",
      version: "1.2.0",
      description: "团队代码检查工具",
      digest: "sha256-v2:validated",
      targetRoot: "/tmp/validation/acme.tools",
      skillCount: 2,
      serverCount: 1,
      dialect: "cursor" as const,
      manifestPath: ".cursor-plugin/plugin.json",
      componentTotals: { total: 23, runnable: 13, needsConfiguration: 0, unsupported: 1, ignored: 5, invalid: 4 },
      processIntents: [
        { componentId: "mcp/browser", kind: "process", summary: "启动本地 browser MCP server" },
        { componentId: "hook/audit", kind: "network", summary: "连接审计端点" },
      ],
      diagnostics: [{ code: "component_invalid", message: "4 个组件无效", severity: "error" as const }],
    }));
    const installPlugin = vi.fn()
      .mockRejectedValueOnce(new Error("PLUGIN_SOURCE_CHANGED: plugin source changed"))
      .mockResolvedValueOnce({
        plugin,
        message: "plugin installed",
      });
    const onNotice = vi.fn();
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => [plugin]),
      pickPluginSource,
      validatePlugin,
      installPlugin,
      listPluginComponents: vi.fn(async () => []),
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={onNotice}
        />,
      );
    });
    await settle();

    await act(async () => buttonByText("已安装插件")?.click());

    expect(document.querySelector(".aw-plugins")).not.toBeNull();
    expect(document.querySelector(".extension-view")).toBeNull();
    const installedPluginCard = document.querySelector<HTMLElement>(".aw-plugin-card");
    expect(installedPluginCard?.textContent).toContain(
      "acme.tools",
    );
    expect(installedPluginCard?.textContent).not.toContain("1.2.0");
    expect(installedPluginCard?.textContent).not.toContain("兼容");
    expect(installedPluginCard?.textContent).not.toContain("部分可用");
    expect(installedPluginCard?.textContent).not.toContain("技能");
    expect(installedPluginCard?.textContent).not.toContain("MCP");
    expect(installedPluginCard?.querySelector(".aw-plugin-card__meta")).toBeNull();
    expect(
      [...(installedPluginCard?.querySelector(".aw-plugin-card__title-row")?.children ?? [])]
        .map((item) => item.textContent),
    ).toEqual(["acme.tools", "Cursor"]);
    expect(
      document.querySelector(".aw-plugin-card__icon .aw-icon--puzzle"),
    ).not.toBeNull();
    expect(buttonByText("Marketplace")).toBeUndefined();

    const searchInput = document.querySelector<HTMLInputElement>(
      '.aw-plugins__search input[placeholder="搜索插件…"]',
    );
    await act(async () => {
      searchInput?.focus();
      if (searchInput) {
        const setter = Object.getOwnPropertyDescriptor(
          HTMLInputElement.prototype,
          "value",
        )?.set;
        setter?.call(searchInput, "missing");
        searchInput.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });
    expect(document.body.textContent).toContain("没有匹配的插件");
    expect(document.querySelector(".aw-plugin-card")).toBeNull();

    await act(async () => buttonByText("安装插件")?.click());
    const installDialog = document.querySelector<HTMLElement>(
      ".aw-plugin-install-dialog[role=\"dialog\"]",
    );
    const dropzone = installDialog?.querySelector<HTMLButtonElement>(
      ".aw-plugin-install-dialog__dropzone",
    );
    expect(installDialog?.textContent).toContain(
      "兼容 Codex、Cursor、Claude Code、Agent Plugins 格式",
    );
    expect(installDialog?.textContent).toContain("点击选择或拖放插件 ZIP");
    expect(installDialog?.textContent).not.toContain("检查插件");
    expect(installDialog?.textContent).not.toContain("同名时替换");
    expect(installDialog?.textContent).not.toContain("插件可访问你的电脑");
    expect(installDialog?.querySelectorAll('input[type="checkbox"]')).toHaveLength(0);
    expect(buttonByText("安装")?.disabled).toBe(true);

    await act(async () => dropzone?.click());
    await settle();
    expect(pickPluginSource).toHaveBeenCalledWith(undefined);
    expect(validatePlugin).toHaveBeenLastCalledWith({
      kind: "local_path",
      path: "/tmp/acme-plugin.zip",
    });
    expect(dropzone?.dataset.state).toBe("valid");
    expect(installDialog?.textContent).toContain("插件 ZIP 格式正确");
    expect(installDialog?.textContent).toContain("acme.tools · v1.2.0");
    expect(installDialog?.textContent).not.toContain("component_invalid");
    expect(buttonByText("安装")?.disabled).toBe(false);

    await act(async () => dropzone?.click());
    await settle();
    expect(pickPluginSource).toHaveBeenLastCalledWith("/tmp/acme-plugin.zip");
    expect(validatePlugin).toHaveBeenLastCalledWith({
      kind: "local_path",
      path: "/tmp/acme-plugin-v2.zip",
    });
    expect(dropzone?.dataset.state).toBe("valid");

    await act(async () => buttonByText("安装")?.click());
    await settle();
    expect(validatePlugin).toHaveBeenCalledTimes(3);
    expect(dropzone?.dataset.state).toBe("valid");
    expect(buttonByText("安装")?.disabled).toBe(false);

    await act(async () => buttonByText("安装")?.click());
    await settle();
    expect(installPlugin).toHaveBeenCalledWith({
      source: { kind: "local_path", path: "/tmp/acme-plugin-v2.zip" },
      replace: true,
      expectedDigest: "sha256-v2:validated",
    });
    expect(installPlugin).toHaveBeenCalledTimes(2);
    expect(onNotice).toHaveBeenCalledWith("插件已安装，将在新任务中生效");
    expect(document.querySelector(".aw-plugin-console")).not.toBeNull();

    await act(async () => buttonByText("返回插件")?.click());
    await settle();
    expect(document.querySelector(".aw-plugin-console")).toBeNull();
    expect(document.querySelector(".aw-plugins")).not.toBeNull();
  });

  it("separates built-in Plugins, renders the Outlook brand, and enables them in place", async () => {
    const outlook: RuntimePlugin = {
      name: "awaken-outlook",
      displayName: "Outlook 邮箱",
      version: "1.0.0",
      description: "Awaken 官方 Outlook 邮箱插件",
      builtin: true,
      brandIcon: "outlook",
      builtinOrder: 0,
      enabled: false,
      source: { kind: "local_path", path: "/app/builtin-plugins/awaken-outlook" },
      skillCount: 4,
      serverCount: 1,
    };
    const installed: RuntimePlugin = {
      name: "team.plugin",
      description: "团队插件",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/team.plugin.zip" },
      skillCount: 1,
      serverCount: 0,
    };
    let resolveEnable: ((plugin: RuntimePlugin) => void) | undefined;
    const pendingEnable = new Promise<RuntimePlugin>((resolvePromise) => {
      resolveEnable = resolvePromise;
    });
    const enablePlugin = vi.fn(() => pendingEnable);
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => [installed, outlook]),
      enablePlugin,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    expect(buttonByText("内置插件")?.getAttribute("aria-selected")).toBe("true");
    expect(document.querySelector(".aw-plugin-card")?.textContent).toContain("Outlook 邮箱");
    expect(document.querySelector(".aw-plugin-card")?.textContent).toContain("Awaken 内置");
    expect(document.body.textContent).not.toContain("团队插件");
    expect(
      document.querySelector('.aw-plugin-card__icon .aw-plugin-brand-icon[data-brand="outlook"][data-size="card"] img'),
    ).not.toBeNull();

    await act(async () => {
      document.querySelector<HTMLButtonElement>('[role="switch"][aria-label="开启Outlook 邮箱"]')?.click();
      await Promise.resolve();
    });
    expect(
      document.querySelector<HTMLButtonElement>('[role="switch"][aria-label="关闭Outlook 邮箱"]')
        ?.getAttribute("aria-checked"),
    ).toBe("true");
    await act(async () => {
      resolveEnable?.({ ...outlook, enabled: true });
      await pendingEnable;
    });
    await settle();
    expect(enablePlugin).toHaveBeenCalledWith("awaken-outlook");

    await act(async () => buttonByText("已安装插件")?.click());
    expect(document.querySelector(".aw-plugin-card")?.textContent).toContain("team.plugin");
    expect(document.body.textContent).not.toContain("Outlook 邮箱");
  });

  it("renders the cached Plugin catalog and opens details before background requests finish", async () => {
    const outlook: RuntimePlugin = {
      name: "awaken-outlook",
      displayName: "Outlook 邮箱",
      version: "1.0.0",
      description: "Awaken 官方 Outlook 邮箱插件",
      builtin: true,
      brandIcon: "outlook",
      builtinOrder: 0,
      enabled: true,
      source: { kind: "local_path", path: "/app/builtin-plugins/awaken-outlook" },
      skillCount: 4,
      serverCount: 1,
    };
    let resolvePlugins: ((plugins: RuntimePlugin[]) => void) | undefined;
    let resolvePluginDetail: ((plugin: RuntimePlugin) => void) | undefined;
    const pluginRefresh = new Promise<RuntimePlugin[]>((resolve) => {
      resolvePlugins = resolve;
    });
    const pluginDetail = new Promise<RuntimePlugin>((resolve) => {
      resolvePluginDetail = resolve;
    });
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(() => pluginRefresh),
      getPlugin: vi.fn(() => pluginDetail),
      listPluginComponents: vi.fn(async () => []),
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          initialPlugins={[outlook]}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });

    expect(document.querySelector(".aw-plugin-card")?.textContent).toContain("Outlook 邮箱");
    expect(document.body.textContent).not.toContain("正在加载插件");

    await act(async () => {
      document.querySelector<HTMLButtonElement>('[aria-label="查看插件 Outlook 邮箱"]')?.click();
    });
    expect(document.querySelector(".aw-plugin-console h1")?.textContent).toBe("Outlook 邮箱");

    await act(async () => {
      resolvePlugins?.([outlook]);
      resolvePluginDetail?.(outlook);
      await Promise.all([pluginRefresh, pluginDetail]);
    });
    await settle();
  });

  it("routes a Plugin ZIP selected in Skills to the Plugin installer", async () => {
    const onNotice = vi.fn();
    const pluginValidation = {
      valid: true,
      source: { kind: "local_path" as const, path: "/tmp/example-plugin.zip" },
      name: "example-plugin",
      version: "1.0.0",
      digest: "sha256-v2:plugin",
      targetRoot: "/tmp/validation/example-plugin",
      skillCount: 1,
      serverCount: 0,
      diagnostics: [],
    };
    const installSkill = vi.fn();
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      resolveDroppedPluginSource: vi.fn(async () => ({
        canceled: false as const,
        path: "/tmp/example-plugin.zip",
      })),
      validatePlugin: vi.fn(async () => pluginValidation),
      installSkill,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <RoutedInstallHarness
          initialSection="skills"
          runtimeClient={runtimeClient}
          onNotice={onNotice}
        />,
      );
    });
    await settle();
    await act(async () => buttonByText("安装 Skill")?.click());

    const input = document.querySelector<HTMLInputElement>(
      '.extension-dropzone input[type="file"]',
    );
    const pluginZip = new File(["zip"], "example-plugin.zip", {
      type: "application/zip",
    });
    Object.defineProperty(input, "files", {
      configurable: true,
      value: { 0: pluginZip, length: 1, item: () => pluginZip },
    });
    await act(async () => input?.dispatchEvent(new Event("change", { bubbles: true })));
    await settle();

    const pluginDialog = document.querySelector<HTMLElement>(
      '.aw-plugin-install-dialog[role="dialog"]',
    );
    expect(pluginDialog).not.toBeNull();
    expect(pluginDialog?.textContent).toContain("插件 ZIP 格式正确");
    expect(pluginDialog?.textContent).toContain("example-plugin · v1.0.0");
    expect(installSkill).not.toHaveBeenCalled();
    expect(onNotice).toHaveBeenCalledWith("已识别为插件包，已切换到插件安装");
  });

  it("routes a Skill ZIP selected in Plugins and installs it only in the chosen Skill scope", async () => {
    const onNotice = vi.fn();
    const installedSkill: RuntimeSkill = {
      ...projectSkill,
      id: "daily-report",
      catalogEntryId: "catalog-daily-report",
      name: "daily-report",
      displayName: "日报",
      rootDir: `${projectRoot}/.awaken/skills/daily-report`,
      skillFilePath: `${projectRoot}/.awaken/skills/daily-report/SKILL.md`,
    };
    const installSkill = vi.fn(async () => ({
      skill: installedSkill,
      path: installedSkill.rootDir ?? "",
      message: "project skill installed",
    }));
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      pickPluginSource: vi.fn(async () => ({
        canceled: false as const,
        path: "/tmp/daily-report.zip",
      })),
      validatePlugin: vi.fn(async () => {
        throw new Error("SKILL_PACKAGE_SELECTED: use the Skill installer");
      }),
      installSkill,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <RoutedInstallHarness
          initialSection="plugins"
          runtimeClient={runtimeClient}
          onNotice={onNotice}
        />,
      );
    });
    await settle();
    await act(async () => buttonByText("安装插件")?.click());
    await act(async () => {
      document.querySelector<HTMLButtonElement>(
        ".aw-plugin-install-dialog__dropzone",
      )?.click();
    });
    await settle();

    const skillDialog = Array.from(document.querySelectorAll<HTMLElement>('[role="dialog"]'))
      .find((dialog) => dialog.textContent?.includes("安装 Skill"));
    expect(skillDialog).not.toBeNull();
    expect(skillDialog?.textContent).toContain("daily-report.zip");
    expect(onNotice).toHaveBeenCalledWith("已识别为 Skill 包，请确认安装范围");

    await act(async () => buttonByText("安装")?.click());
    await settle();
    expect(installSkill).toHaveBeenCalledWith({
      fileName: "daily-report.zip",
      contentBase64: undefined,
      sourcePath: "/tmp/daily-report.zip",
      scope: "project",
      projectRoot,
    });
    expect(onNotice).toHaveBeenCalledWith("已安装 日报");
    expect(document.querySelector(".aw-plugin-install-dialog")).toBeNull();
  });

  it("sanitizes direct Plugin validation errors before rendering them", async () => {
    const validatePlugin = vi.fn(async () => {
      throw new Error(
        "Error invoking remote method 'awaken:validate-plugin': Error: PLUGIN_INVALID: open /Users/test/private/plugin.json",
      );
    });
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      pickPluginSource: vi.fn(async () => ({
        canceled: false as const,
        path: "/missing/plugin.zip",
      })),
      validatePlugin,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    await act(async () => buttonByText("安装插件")?.click());
    const dropzone = document.querySelector<HTMLButtonElement>(
      ".aw-plugin-install-dialog__dropzone",
    );
    await act(async () => dropzone?.click());
    await settle();

    expect(validatePlugin).toHaveBeenCalledWith({
      kind: "local_path",
      path: "/missing/plugin.zip",
    });
    expect(dropzone?.dataset.state).toBe("invalid");
    expect(document.body.textContent).toContain("插件 ZIP 格式不符合要求");
    expect(document.body.textContent).not.toContain("PLUGIN_INVALID");
    expect(document.body.textContent).not.toContain("invoking remote method");
    expect(document.body.textContent).not.toContain("/Users/");
  });

  it("keeps Plugin Console Skill inventory, direct preview, toggle, and exact use action without a user-copy entry", async () => {
    const plugin: RuntimePlugin = {
      name: "console.plugin",
      version: "1.0.0",
      description: "Plugin Console 回归夹具",
      dialect: "codex",
      compatibilityState: "compatible",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/console.plugin.zip" },
      digest: "digest-console-v1",
      skillCount: 1,
      serverCount: 0,
      componentTotals: {
        total: 1,
        runnable: 1,
        needsConfiguration: 0,
        unsupported: 0,
        ignored: 0,
        invalid: 0,
      },
    };
    const componentState: RuntimePluginComponentState = {
      component: {
        logicalId: "skills/private-review",
        logicalIdEncoded: "c2tpbGxzL3ByaXZhdGUtcmV2aWV3",
        revisionId: "revision-skill-v1",
        kind: "skill",
        displayName: "插件私有复核",
        description: "只通过 Plugin Console 管理",
        sourceDialect: "codex",
        sourcePath: "skills/private-review/SKILL.md",
        mappingState: "native",
        validation: "valid",
        diagnostics: [
          { code: "skill_notice", message: "测试诊断", severity: "info" },
        ],
        skill: { path: "skills/private-review" },
      },
      desiredState: "enabled",
      configurationState: "ready",
      runtimeState: "loaded",
      integrityState: "intact",
      mappingState: "native",
      switchable: true,
    };
    const componentDetail: RuntimePluginComponentDetail = {
      state: componentState,
      preview: "# 插件 Skill 预览",
      source: "---\nname: private-review\ndescription: review\n---",
      sourceEditable: false,
      resources: ["SKILL.md", "references/checklist.md"],
      properties: { skill: { packagePath: "skills/private-review" } },
    };
    const onUsePlugin = vi.fn();
    const onUsePluginComponent = vi.fn();
    const onNotice = vi.fn();
    const setPluginComponentState = vi.fn(async () => ({ plugin, revision: 2 }));
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => [plugin]),
      getPlugin: vi.fn(async () => plugin),
      listPluginComponents: vi.fn(async () => [componentState]),
      getPluginComponent: vi.fn(async () => componentDetail),
      setPluginComponentState,
      getPluginConfiguration: vi.fn(async () => ({ fields: [], revision: 1 })),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={onNotice}
          onUsePlugin={onUsePlugin}
          onUsePluginComponent={onUsePluginComponent}
        />,
      );
    });
    await settle();

    await act(async () => buttonByText("已安装插件")?.click());

    await act(async () => {
      document.querySelector<HTMLButtonElement>(".aw-plugin-card__main")?.click();
    });
    await settle();
    expect(document.querySelector(".aw-plugin-console")?.textContent).toContain("技能");
    expect(document.querySelector(".aw-plugin-console")?.textContent).toContain("插件私有复核");

    await act(async () => {
      document.querySelector<HTMLButtonElement>(".aw-plugin-component-row__main")?.click();
    });
    await settle();
    const componentDialog = document.querySelector<HTMLElement>(
      '.aw-plugin-component-dialog[role="dialog"]',
    );
    expect(componentDialog).not.toBeNull();
    expect(document.querySelector(".aw-plugin-inspector")).toBeNull();
    expect(componentDialog?.textContent).toContain("插件 Skill 预览");
    const useInChatButton = buttonByText("在对话中使用");
    expect(useInChatButton).toBeTruthy();
    expect(useInChatButton?.closest(".aw-dialog__foot")).not.toBeNull();
    expect(useInChatButton?.closest(".aw-dialog__body")).toBeNull();
    expect(componentDialog?.querySelector(".aw-plugin-component-dialog__title small")).toBeNull();
    expect(buttonByText("创建完整用户副本")).toBeUndefined();
    expect(componentDialog?.querySelector('[role="tablist"]')).toBeNull();
    expect(buttonByText("详情")).toBeUndefined();
    expect(buttonByText("预览")).toBeUndefined();
    expect(buttonByText("源文件")).toBeUndefined();
    expect(componentDialog?.textContent).not.toContain("references/checklist.md");
    expect(componentDialog?.textContent).not.toContain("name: private-review");
    expect(componentDialog?.textContent).not.toContain("测试诊断");

    await act(async () => buttonByText("在对话中使用")?.click());
    expect(onUsePlugin).not.toHaveBeenCalled();
    expect(onUsePluginComponent).toHaveBeenCalledWith(plugin, componentState.component, []);

    await act(async () => {
      document.querySelector<HTMLButtonElement>(
        '[role="switch"][aria-label="关闭插件私有复核"]',
      )?.click();
    });
    await settle();
    expect(setPluginComponentState).toHaveBeenCalledWith(
      "console.plugin",
      "skill",
      "c2tpbGxzL3ByaXZhdGUtcmV2aWV3",
      false,
    );

  });

  it("accepts a dropped ZIP and rejects a dropped non-ZIP before Core validation", async () => {
    const resolveDroppedPluginSource = vi
      .fn()
      .mockResolvedValueOnce({ canceled: true as const, error: "请选择 .zip 插件包" })
      .mockResolvedValueOnce({ canceled: false as const, path: "/tmp/dropped-plugin.zip" });
    const validatePlugin = vi.fn(async (source: RuntimePluginSource) => ({
      valid: true,
      source,
      name: "dropped.plugin",
      version: "1.0.0",
      digest: "sha256-v2:dropped",
      targetRoot: "/tmp/validation/dropped.plugin",
      skillCount: 1,
      serverCount: 0,
      diagnostics: [],
    }));
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins: vi.fn(async () => []),
      resolveDroppedPluginSource,
      validatePlugin,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();
    await act(async () => buttonByText("安装插件")?.click());

    const dropzone = document.querySelector<HTMLButtonElement>(
      ".aw-plugin-install-dialog__dropzone",
    );
    const drop = async (file: File): Promise<void> => {
      const event = new Event("drop", { bubbles: true, cancelable: true });
      Object.defineProperty(event, "dataTransfer", {
        value: {
          dropEffect: "none",
          files: {
            0: file,
            length: 1,
            item: () => file,
          },
        },
      });
      await act(async () => {
        dropzone?.dispatchEvent(event);
      });
      await settle();
    };

    const textFile = new File(["not a zip"], "plugin.txt", { type: "text/plain" });
    await drop(textFile);
    expect(resolveDroppedPluginSource).toHaveBeenLastCalledWith(textFile);
    expect(validatePlugin).not.toHaveBeenCalled();
    expect(dropzone?.dataset.state).toBe("invalid");
    expect(document.body.textContent).toContain("请选择 .zip 插件包");

    const zipFile = new File(["zip"], "plugin.zip", { type: "application/zip" });
    await drop(zipFile);
    expect(resolveDroppedPluginSource).toHaveBeenLastCalledWith(zipFile);
    expect(validatePlugin).toHaveBeenCalledWith({
      kind: "local_path",
      path: "/tmp/dropped-plugin.zip",
    });
    expect(dropzone?.dataset.state).toBe("valid");
    expect(document.body.textContent).toContain("插件 ZIP 格式正确");
  });

  it("refreshes an open Plugin catalog after a committed store revision", async () => {
    const before: RuntimePlugin = {
      name: "acme.tools",
      version: "1.0.0",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/acme.tools" },
      digest: "sha256-v2:before",
      skillCount: 0,
      serverCount: 0,
      diagnostics: [{
        code: "plugin_digest_changed",
        message: "installed plugin package digest changed",
        severity: "error",
      }],
    };
    const after: RuntimePlugin = {
      ...before,
      version: "1.1.0",
      description: "Catalog 已刷新",
      digest: "sha256-v2:after",
      skillCount: 1,
      diagnostics: [],
    };
    const listPlugins = vi.fn()
      .mockResolvedValueOnce([before])
      .mockResolvedValue([after]);
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => []),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          onNotice={vi.fn()}
          composerCatalogRevision={1}
        />,
      );
    });
    await settle();
    await act(async () => buttonByText("已安装插件")?.click());
    expect(document.body.textContent).toContain("installed plugin package digest changed");

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          onNotice={vi.fn()}
          composerCatalogRevision={2}
        />,
      );
    });
    await settle();

    expect(listPlugins).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("Catalog 已刷新");
    expect(document.body.textContent).not.toContain("1.1.0");
    expect(document.body.textContent).not.toContain("installed plugin package digest changed");
  });

  it("catches up a Plugin catalog revision published while another section is open", async () => {
    const listPlugins = vi.fn(async () => []);
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => []),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          onNotice={vi.fn()}
          composerCatalogRevision={1}
        />,
      );
    });
    await settle();
    expect(listPlugins).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          onNotice={vi.fn()}
          composerCatalogRevision={2}
        />,
      );
    });
    await settle();
    expect(listPlugins).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          onNotice={vi.fn()}
          composerCatalogRevision={2}
        />,
      );
    });
    await settle();
    expect(listPlugins).toHaveBeenCalledTimes(2);
  });

  it("does not refresh an open Plugin catalog when its revision is unchanged", async () => {
    const listPlugins = vi.fn(async () => []);
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => []),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      listPlugins,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          onNotice={vi.fn()}
          composerCatalogRevision={3}
        />,
      );
    });
    await settle();
    expect(listPlugins).toHaveBeenCalledTimes(1);

    await act(async () => {
      root.render(
        <ExtensionsView
          section="plugins"
          desktopReady
          runtimeClient={runtimeClient}
          onNotice={vi.fn()}
          composerCatalogRevision={3}
        />,
      );
    });
    await settle();
    expect(listPlugins).toHaveBeenCalledTimes(1);
  });

  it("keeps Plugin Skills out of every Skills view, search result, count, and preview state", async () => {
    const getSkillPreview = vi.fn();
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => [pluginSkill]),
      listPlugins: vi.fn(async () => []),
      getSkillPreview,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    expect(document.body.textContent).not.toContain("插件私有复核");
    await act(async () => buttonByText("项目 Skills")?.click());
    expect(
      document.querySelector('[aria-label="打开营销项目 项目 Skills"]')?.textContent,
    ).toContain("0 Skill");

    const search = document.querySelector<HTMLInputElement>(
      '.extension-toolbar input[placeholder="搜索..."]',
    );
    await setInputValue(search, "插件私有复核");
    for (const label of [
      "Awaken Skills",
      "项目 Skills",
      "全局 Skills",
      "Agents Skills",
    ]) {
      await act(async () => buttonByText(label)?.click());
      expect(document.body.textContent).not.toContain("插件私有复核");
      expect(document.body.textContent).not.toContain("其他来源");
      expect(document.body.textContent).toContain("未找到匹配的 Skill");
    }
    expect(document.querySelector('[aria-label="查看 插件私有复核 的 Skill 包"]')).toBeNull();
    expect(getSkillPreview).not.toHaveBeenCalled();
  });

  it("opens a project window, previews SKILL.md, and deletes through confirmation", async () => {
    let deleted = false;
    const listSkills = vi.fn(async () =>
      deleted ? [bundledSkill] : [projectSkill, bundledSkill],
    );
    const getSkillPreview = vi.fn(async () => ({
      frontmatterRaw: "---\nname: project-report\n---",
      bodyMarkdown: "# 项目报告正文",
      resources: [
        { path: "SKILL.md", kind: "file", sizeBytes: 128 },
        { path: "references", kind: "directory", sizeBytes: 0 },
      ],
    }));
    const deleteSkill = vi.fn(async () => {
      deleted = true;
      return {
        id: projectSkill.id,
        sourceOrigin: "project",
        originalPath: projectSkill.rootDir!,
        trashedPath: "/Users/test/.Trash/project-report",
        message: "deleted",
      };
    });
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills,
      listPlugins: vi.fn(async () => []),
      getSkillPreview,
      deleteSkill,
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    const bundledCard = document.querySelector<HTMLElement>(".aw-skill-card");
    expect(bundledCard?.textContent).toContain("内置报告");
    expect(bundledCard?.querySelector(".aw-switch")?.getAttribute("data-size")).toBe(
      "compact",
    );
    expect(bundledCard?.querySelector(".aw-skill-card__title-row")?.textContent).toContain(
      "内置报告Awaken 内置",
    );
    expect(bundledCard?.textContent).not.toContain("可预览 SKILL.md");
    expect(bundledCard?.querySelector('[aria-label^="删除"]')).toBeNull();
    expect(document.querySelector(".extension-skill-card")).toBeNull();

    await act(async () => buttonByText("项目 Skills")?.click());
    const folder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开营销项目 项目 Skills"]',
    );
    expect(folder).not.toBeNull();
    expect(folder?.dataset.size).toBe("dense");
    expect(folder?.getAttribute("title")).toBeNull();
    expect(document.querySelector(".skill-project-panel")).toBeNull();

    await act(async () => folder?.click());
    const projectDialog = document.querySelector('[role="dialog"]');
    expect(projectDialog?.textContent).toContain("营销项目");
    expect(projectDialog?.textContent).toContain("项目报告");
    expect(
      projectDialog?.querySelector(".aw-skill-card .aw-switch")?.getAttribute("data-size"),
    ).toBe("compact");
    expect(
      projectDialog?.querySelector(".aw-project-skills-dialog__heading strong"),
    ).toBeNull();
    const folderSummary = projectDialog?.querySelector<HTMLButtonElement>(
      '[aria-label="筛选 1 个项目文件夹"]',
    );
    expect(folderSummary?.textContent).toContain("1 个项目文件夹");
    expect(folderSummary?.textContent).toContain("主：marketing");
    expect(projectDialog?.textContent).not.toContain(projectRoot);
    await act(async () => folderSummary?.click());
    const folderPopover = document.querySelector<HTMLElement>(
      ".aw-project-skills-dialog__folder-popover",
    );
    expect(folderPopover?.textContent).toContain("marketing");
    expect(folderPopover?.textContent).not.toContain(projectRoot);
    expect(folderPopover?.querySelector(".aw-project-skills-dialog__path")).toBeNull();
    await act(async () => folderSummary?.click());
    const projectBackdrop = projectDialog?.closest(".aw-dialog-backdrop");

    const previewButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 项目报告 的 Skill 包"]',
    );
    await act(async () => previewButton?.click());
    await settle();
    const previewDialog = document.querySelector<HTMLElement>(
      '.aw-project-skills-dialog--preview[role="dialog"]',
    );
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(previewDialog).toBe(projectDialog);
    expect(previewDialog?.closest(".aw-dialog-backdrop")).toBe(projectBackdrop);
    expect(document.querySelector(".aw-skill-preview")).toBeNull();
    expect(getSkillPreview).toHaveBeenCalledWith({
      catalogEntryId: "catalog-project-report",
      projectRoot,
    });
    expect(document.body.textContent).toContain("项目报告正文");
    expect(previewDialog?.querySelector(".aw-skill-preview-summary")?.textContent).toContain(
      "项目报告项目",
    );
    expect(previewDialog?.querySelector(".aw-skill-preview__delete")).not.toBeNull();
    assertSplitPreviewCards(previewDialog as ParentNode);

    const previewClose = document.querySelector<HTMLButtonElement>(
      '.aw-project-skills-dialog--preview button[aria-label="关闭"]',
    );
    await act(async () => previewClose?.click());
    const returnedProjectDialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(returnedProjectDialog).toBe(projectDialog);
    expect(returnedProjectDialog?.closest(".aw-dialog-backdrop")).toBe(projectBackdrop);
    expect(returnedProjectDialog?.textContent).toContain("项目报告");
    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "查看 项目报告 的 Skill 包",
    );

    expect(document.querySelector('[aria-label="删除 项目报告"]')).toBeNull();
    const reopenedPreviewButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 项目报告 的 Skill 包"]',
    );
    await act(async () => reopenedPreviewButton?.click());
    await settle();
    const deleteButton = document.querySelector<HTMLButtonElement>(
      ".aw-skill-preview__delete",
    );
    await act(async () => deleteButton?.click());
    expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
    expect(document.querySelector('[role="dialog"]')).toBe(projectDialog);
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "确定要删除 Skill",
    );

    await act(async () => buttonByText("确认删除")?.click());
    await settle();
    expect(deleteSkill).toHaveBeenCalledWith({
      id: "project-report",
      sourceOrigin: "project",
      projectRoot,
    });
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "这个项目还没有 Skills",
    );
  });

  it("shows project entities immediately and aggregates Skills from every attached folder", async () => {
    const secondaryRoot = "/workspace/shared-data";
    const emptyRoot = "/workspace/legal";
    const secondarySkill: RuntimeSkill = {
      ...projectSkill,
      id: "secondary-analysis",
      catalogEntryId: "catalog-secondary-analysis",
      name: "secondary-analysis",
      displayName: "次文件夹分析",
      description: "读取次文件夹数据",
      rootDir: `${secondaryRoot}/.awaken/skills/secondary-analysis`,
      skillFilePath: `${secondaryRoot}/.awaken/skills/secondary-analysis/SKILL.md`,
      projectPath: secondaryRoot,
    };
    const multiFolderProject: RuntimeProject = {
      ...runtimeProject,
      projectId: "project-quarterly",
      displayName: "季度经营分析",
      folders: [
        { path: projectRoot, role: "user", isPrimary: true },
        { path: secondaryRoot, role: "user", isPrimary: false },
        { path: emptyRoot, role: "user", isPrimary: false },
      ],
    };
    const folderlessProject: RuntimeProject = {
      ...runtimeProject,
      projectId: "project-empty",
      workspaceRoot: "",
      folders: [],
      displayName: "待规划项目",
    };
    const listSkills = vi.fn(async () => [projectSkill, secondarySkill]);
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [multiFolderProject, folderlessProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills,
      listPlugins: vi.fn(async () => []),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    expect(listSkills).toHaveBeenCalledWith([projectRoot, secondaryRoot, emptyRoot]);
    await act(async () => buttonByText("项目 Skills")?.click());
    const projectFolder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开季度经营分析 项目 Skills"]',
    );
    const emptyProjectFolder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开待规划项目 项目 Skills"]',
    );
    expect(projectFolder?.textContent).toContain("2 Skill");
    expect(emptyProjectFolder?.textContent).toContain("0 Skill");

    await act(async () => projectFolder?.click());
    const projectDialog = document.querySelector<HTMLElement>('[role="dialog"]');
    expect(projectDialog?.textContent).toContain("季度经营分析 Skills");
    const folderSummary = projectDialog?.querySelector<HTMLButtonElement>(
      '[aria-label="筛选 3 个项目文件夹"]',
    );
    expect(folderSummary?.textContent).toContain("3 个项目文件夹");
    expect(folderSummary?.textContent).toContain("主：marketing");
    expect(projectDialog?.textContent).not.toContain(projectRoot);
    expect(projectDialog?.textContent).not.toContain(secondaryRoot);
    expect(projectDialog?.textContent).toContain("项目报告marketing");
    expect(projectDialog?.textContent).toContain("次文件夹分析shared-data");
    expect(projectDialog?.textContent).not.toContain("主文件夹 · marketing");
    expect(projectDialog?.textContent).not.toContain("文件夹 · shared-data");

    await act(async () => folderSummary?.click());
    const folderPopover = document.querySelector<HTMLElement>(
      ".aw-project-skills-dialog__folder-popover",
    );
    const folderPopoverHeading = folderPopover?.querySelector(
      ".aw-project-skills-dialog__folder-popover-heading",
    );
    expect(folderPopoverHeading?.firstElementChild?.textContent).toBe("项目文件夹");
    expect(
      folderPopoverHeading?.querySelector(
        ".aw-project-skills-dialog__folder-popover-count",
      )?.textContent,
    ).toBe("3");
    expect(folderPopover?.textContent).toContain("marketing主文件夹");
    expect(folderPopover?.textContent).toContain("shared-data");
    expect(folderPopover?.textContent).toContain("legal");
    expect(folderPopover?.textContent).not.toContain(projectRoot);
    expect(folderPopover?.textContent).not.toContain(secondaryRoot);
    expect(folderPopover?.textContent).not.toContain(emptyRoot);

    const emptyFolderOption = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 legal 的 Skills"]',
    );
    expect(emptyFolderOption?.disabled).toBe(false);
    await act(async () => emptyFolderOption?.click());
    expect(folderSummary?.textContent).toContain("当前：legal");
    expect(projectDialog?.textContent).toContain("0 Skills");
    expect(projectDialog?.textContent).toContain("这个文件夹还没有 Skills");
    expect(projectDialog?.textContent).not.toContain("项目报告marketing");
    expect(projectDialog?.textContent).not.toContain("次文件夹分析shared-data");

    await act(async () => folderSummary?.click());
    const allFoldersOption = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看全部文件夹的 Skills"]',
    );
    await act(async () => allFoldersOption?.click());
    expect(folderSummary?.textContent).toContain("主：marketing");
    expect(projectDialog?.textContent).toContain("2 Skills");
    expect(projectDialog?.textContent).toContain("项目报告marketing");
    expect(projectDialog?.textContent).toContain("次文件夹分析shared-data");
  });

  it("shows all Agents Skills directly without collapsible source groups", async () => {
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills: vi.fn(async () => [globalAgentsSkill, projectAgentsSkill]),
      listPlugins: vi.fn(async () => []),
      getSkillPreview: vi.fn(async () => ({
        frontmatterRaw: "---\nname: project-review\n---",
        bodyMarkdown: "# 项目代码复核",
      })),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    await act(async () => buttonByText("Agents Skills")?.click());
    const agentsSection = document.querySelector<HTMLElement>(
      'section[aria-label="Agents Skills"]',
    );
    const cards = agentsSection?.querySelectorAll(".aw-skill-card");

    expect(cards).toHaveLength(2);
    expect(agentsSection?.textContent).toContain("DWS");
    expect(agentsSection?.textContent).toContain("项目代码复核");
    expect(agentsSection?.querySelector("[aria-expanded]")).toBeNull();
    expect(agentsSection?.querySelector(".skill-project-group")).toBeNull();
    expect(agentsSection?.textContent).not.toContain("全局 Agents Skills");
    expect(agentsSection?.textContent).not.toContain(projectRoot);
  });

  it("syncs an external project Skill deletion and closes its stale preview", async () => {
    let skillExistsOnDisk = true;
    const listSkills = vi.fn(async () =>
      skillExistsOnDisk ? [projectSkill] : [],
    );
    const runtimeClient = {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: projectRoot })),
      listProjects: vi.fn(async () => [runtimeProject]),
      listConnectors: vi.fn(async () => []),
      listCliConnectors: vi.fn(async () => []),
      listMcpServers: vi.fn(async () => []),
      listSkills,
      listPlugins: vi.fn(async () => []),
      getSkillPreview: vi.fn(async () => ({
        frontmatterRaw: "---\nname: project-report\n---",
        bodyMarkdown: "# 项目报告正文",
      })),
      subscribeCliConnectorState: vi.fn(() => () => undefined),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <ExtensionsView
          section="skills"
          desktopReady
          runtimeClient={runtimeClient}
          activeWorkspaceRoot={projectRoot}
          onNotice={vi.fn()}
        />,
      );
    });
    await settle();

    await act(async () => buttonByText("项目 Skills")?.click());
    const folder = document.querySelector<HTMLButtonElement>(
      '[aria-label="打开营销项目 项目 Skills"]',
    );
    await act(async () => folder?.click());
    const projectDialog = document.querySelector<HTMLElement>('[role="dialog"]');
    const previewButton = document.querySelector<HTMLButtonElement>(
      '[aria-label="查看 项目报告 的 Skill 包"]',
    );
    await act(async () => previewButton?.click());
    await settle();
    expect(projectDialog?.textContent).toContain("项目报告正文");

    skillExistsOnDisk = false;
    await act(async () => {
      await new Promise((resolve) =>
        setTimeout(resolve, SKILL_CATALOG_SYNC_INTERVAL_MS + 100),
      );
    });
    await settle();

    expect(listSkills.mock.calls.length).toBeGreaterThan(1);
    expect(document.querySelector('[role="dialog"]')).toBe(projectDialog);
    expect(projectDialog?.textContent).not.toContain("项目报告正文");
    expect(projectDialog?.textContent).toContain("这个项目还没有 Skills");
  });
});
