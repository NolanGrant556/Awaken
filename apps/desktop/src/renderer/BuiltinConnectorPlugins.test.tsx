// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CliConnectorView, CliConnectorStateEvent, CliProviderId } from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import { ExtensionsView, type ExtensionSection } from "./ExtensionsView";

function view(providerId: CliProviderId, overrides: Partial<CliConnectorView["readiness"]> = {}): CliConnectorView {
  return {
    canUninstallManaged: true,
    providerId, displayName: providerId === "lark-cli" ? "飞书" : "钉钉",
    readiness: {
      providerId, activation: "enabled", install: "installed", executable: "available",
      configuration: "ready", authentication: "authorized", overall: "ready",
      account: { displayName: `${providerId} account`, organization: "Existing organization" },
      updatedAt: "2026-09-14T00:00:00Z", ...overrides,
    },
  };
}
function button(label: string): HTMLButtonElement {
  const scope = document.querySelector('[role="dialog"]') ?? document;
  const found = [...scope.querySelectorAll<HTMLButtonElement>("button")].find(el => el.getAttribute("aria-label") === label || el.textContent?.trim() === label);
  expect(found, label).toBeTruthy();
  return found!;
}
async function click(label: string) { await act(async () => button(label).click()); }

describe("built-in DingTalk and Feishu entry migration", () => {
  let root: Root;
  let container: HTMLDivElement;
  let stateListener: (event: CliConnectorStateEvent) => void;
  let views: CliConnectorView[];
  let client: ReturnType<typeof makeClient>;
  function makeClient() {
    return {
      getRuntimeInfo: vi.fn(async () => ({ defaultWorkspace: "/fixture" })),
      listProjects: vi.fn(async () => []), listSkills: vi.fn(async () => []),
      listConnectors: vi.fn(async () => views.map(v => ({
        id: v.providerId, kind: "cli", displayName: v.displayName, category: "productivity",
        state: "connected", sourceOrigin: "builtin", authType: "cli", cliReadiness: v.readiness,
      }))),
      listCliConnectors: vi.fn(async () => views), listMcpServers: vi.fn(async () => []),
      listPlugins: vi.fn(async () => [{ name: "awaken-outlook", displayName: "Outlook 邮箱", builtin: true, enabled: true, builtinOrder: 0 }]),
      subscribeCliConnectorState: vi.fn((callback: typeof stateListener) => { stateListener = callback; return vi.fn(); }),
      refreshCliConnector: vi.fn(async (providerId: CliProviderId) => views.find(v => v.providerId === providerId)!),
      installCliConnector: vi.fn(async (providerId: CliProviderId) => view(providerId, { install: "installing", authentication: "required", overall: "working" })),
      cancelCliInstall: vi.fn(async (providerId: CliProviderId) => view(providerId, { install: "missing", authentication: "required", overall: "not_ready" })),
      retryCliAuth: vi.fn(async (providerId: CliProviderId): Promise<CliConnectorView> => ({ ...view(providerId, { authentication: "in_progress", overall: "working" }), authSession: { id: "fixture-auth", provider: providerId, phase: "waiting_user" } })),
      cancelCliAuth: vi.fn(async (providerId: CliProviderId) => view(providerId, { authentication: "required", overall: "action_required", account: undefined })),
      logoutCliConnector: vi.fn(async (providerId: CliProviderId) => view(providerId, { authentication: "required", overall: "action_required", account: undefined })),
      uninstallManagedCli: vi.fn(async (providerId: CliProviderId, _removeSkills?: boolean) => {
        const next = { ...view(providerId, { install: "missing", executable: "missing", authentication: "unknown", account: undefined }), canUninstallManaged: false, uninstallMessage: "Awaken 管理的运行组件已卸载。账号已退出登录。" };
        views = views.map(v => v.providerId === providerId ? next : v);
        return next;
      }),
      getPlugin: vi.fn(), enablePlugin: vi.fn(), disablePlugin: vi.fn(),
    };
  }
  async function render(section: ExtensionSection = "plugins") {
    await act(async () => root.render(<ExtensionsView section={section} desktopReady runtimeClient={client as unknown as RuntimeClient} onNotice={vi.fn()} />));
  }
  async function emit(connector: CliConnectorView) {
    views = views.map(v => v.providerId === connector.providerId ? connector : v);
    await act(async () => stateListener({ connector }));
  }
  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div"); document.body.append(container); root = createRoot(container);
    views = [view("dingtalk-workspace"), view("lark-cli")]; client = makeClient();
  });
  afterEach(async () => { await act(async () => root.unmount()); document.body.replaceChildren(); vi.restoreAllMocks(); });

  it("shows connected host entries beside Outlook without changing activation or credentials", async () => {
    await render();
    expect([...container.querySelectorAll(".aw-plugin-card__name")].map(el => el.textContent)).toEqual(["Outlook 邮箱", "钉钉", "飞书"]);
    expect(container.querySelectorAll(".aw-plugin-card__connection[data-connected=true]")).toHaveLength(2);
    expect(container.querySelectorAll(".aw-plugin-card__brand-image")).toHaveLength(2);
    expect(container.textContent).not.toMatch(/CLI/);
    await click("查看插件 钉钉");
    expect(client.refreshCliConnector).toHaveBeenCalledWith("dingtalk-workspace");
    expect(document.body.textContent).toContain("dingtalk-workspace account");
    await click("关闭"); await click("打开 飞书 连接设置");
    expect(client.refreshCliConnector).toHaveBeenLastCalledWith("lark-cli");
    expect(document.body.textContent).toContain("lark-cli account");
    expect(client.getPlugin).not.toHaveBeenCalled(); expect(client.enablePlugin).not.toHaveBeenCalled();
    expect(client.installCliConnector).not.toHaveBeenCalled(); expect(client.retryCliAuth).not.toHaveBeenCalled();
    expect(client.logoutCliConnector).not.toHaveBeenCalled();
    await click("关闭"); await click("已安装插件");
    expect(container.querySelectorAll(".aw-plugin-card")).toHaveLength(0);
  });

  it.each(["dingtalk-workspace", "lark-cli"] as const)("uninstalls %s after logout with explicit skill selection", async (providerId) => {
    await render(); await click(`查看插件 ${view(providerId).displayName}`);
    await click(`取消登录 ${view(providerId).displayName}`);
    await click("卸载");
    expect(document.body.textContent).toContain("先退出当前账号");
    expect(document.activeElement).toBe(button("取消"));
    await click("取消");
    expect(document.activeElement).toBe(button("卸载"));
    expect(client.uninstallManagedCli).not.toHaveBeenCalled();
    await click("卸载");
    await act(async () => document.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click());
    await click("确认卸载");
    expect(client.uninstallManagedCli).toHaveBeenCalledWith(providerId, true);
    expect(button("安装")).toBeTruthy();
    expect(document.body.textContent).toContain("运行组件已卸载");
    expect([...document.querySelectorAll("button")].some(el => el.textContent === "卸载")).toBe(false);
    expect(client.listSkills).toHaveBeenCalled();
  });

  it("keeps uninstall failures in confirmation and allows retry without deleting skills by default", async () => {
    client.uninstallManagedCli.mockRejectedValueOnce(new Error("目录权限不足"));
    await render(); await click("查看插件 钉钉"); await click("卸载"); await click("确认卸载");
    expect(document.querySelector('[role="alert"]')?.textContent).toContain("目录权限不足");
    expect(button("确认卸载")).toBeTruthy();
    await click("确认卸载");
    expect(client.uninstallManagedCli).toHaveBeenLastCalledWith("dingtalk-workspace", false);
    expect(button("安装")).toBeTruthy();
  });

  it("does not offer uninstall for another product's installation", async () => {
    views = views.map(v => ({ ...v, canUninstallManaged: false }));
    await render(); await click("查看插件 钉钉");
    expect([...document.querySelectorAll("button")].some(el => el.textContent === "卸载")).toBe(false);
  });

  it("removes the old CLI tab and cards from Connectors", async () => {
    await render("connectors");
    expect(container.textContent).not.toMatch(/CLI|钉钉|飞书/);
    expect(container.querySelector('[role="tablist"]')).toBeNull();
    expect(button("配置 MCP")).toBeTruthy();
  });

  it.each(["dingtalk-workspace", "lark-cli"] as const)("preserves install progress and explicit cancellation for %s", async (providerId) => {
    views = views.map(v => v.providerId === providerId ? view(providerId, { install: "missing", executable: "missing", authentication: "unknown", overall: "not_ready" }) : v);
    await render(); await click(`查看插件 ${view(providerId).displayName}`); await click("安装");
    expect(client.installCliConnector).toHaveBeenCalledWith(providerId);
    const installing = { ...view(providerId, { install: "installing", authentication: "required", overall: "working" }), installProgress: { phase: "downloading_skills" as const, percent: 72, label: "正在下载官方 Skills" } };
    await emit(installing);
    expect(document.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow")).toBe("72");
    expect(document.body.textContent).not.toMatch(/CLI|Skills/);
    await click("关闭"); expect(client.cancelCliInstall).not.toHaveBeenCalled();
    await click(`查看插件 ${view(providerId).displayName}`);
    expect(document.querySelector('[role="progressbar"]')?.getAttribute("aria-valuenow")).toBe("72");
    await click("取消安装"); expect(client.cancelCliInstall).toHaveBeenCalledWith(providerId);
    expect(button("安装")).toBeTruthy();
  });

  it.each(["dingtalk-workspace", "lark-cli"] as const)("preserves authorization, close-to-cancel, completion and logout for %s", async (providerId) => {
    views = views.map(v => v.providerId === providerId ? view(providerId, { authentication: "required", overall: "action_required", account: undefined }) : v);
    await render(); await click(`查看插件 ${view(providerId).displayName}`); await click("重新授权");
    expect(client.retryCliAuth).toHaveBeenCalledWith(providerId);
    expect(button("取消授权")).toBeTruthy();
    await click("关闭"); expect(client.cancelCliAuth).toHaveBeenCalledWith(providerId);
    await emit(view(providerId, { authentication: "required", overall: "action_required" }));
    await click(`查看插件 ${view(providerId).displayName}`); await click("重新授权");
    await emit(view(providerId));
    expect(document.body.textContent).toContain(`已连接到 ${view(providerId).displayName}`);
    await click(`取消登录 ${view(providerId).displayName}`);
    expect(client.logoutCliConnector).toHaveBeenCalledWith(providerId);
    expect(button("重新授权")).toBeTruthy();
    expect(document.querySelector(".aw-cli-connector-detail__account")).toBeNull();
  });

  it("keeps missing readiness discoverable and reports refresh failures in the dialog", async () => {
    views = []; client.refreshCliConnector.mockRejectedValue(new Error("CLI 状态探测失败"));
    await render(); await click("查看插件 飞书");
    expect(document.querySelector('[role="alert"]')?.textContent).toBe("运行组件 状态探测失败");
    await click("关闭");
    client.refreshCliConnector.mockResolvedValue(view("lark-cli"));
    await click("查看插件 飞书");
    expect(document.body.textContent).toContain("已连接到 飞书");
  });

  it("does not display a late refresh error in another provider dialog", async () => {
    let reject!: (reason: Error) => void;
    client.refreshCliConnector.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; }));
    await render(); await click("查看插件 钉钉"); await click("关闭"); await click("查看插件 飞书");
    await act(async () => reject(new Error("old provider failure")));
    expect(document.body.textContent).not.toContain("old provider failure");
    expect(document.body.textContent).toContain("已连接到 飞书");
  });
  it("keeps newer authorization events when an earlier refresh completes late", async () => {
    let resolve!: (value: CliConnectorView) => void;
    client.refreshCliConnector.mockImplementationOnce(() => new Promise(done => { resolve = done; }));
    await render(); await click("查看插件 飞书");
    await emit(view("lark-cli", { authentication: "required", overall: "action_required", account: undefined }));
    await act(async () => resolve(view("lark-cli")));
    expect(button("重新授权")).toBeTruthy();
    expect(document.querySelector(".aw-cli-connector-detail__account")).toBeNull();
    expect(container.querySelectorAll('.aw-plugin-card__connection[data-connected="true"]')).toHaveLength(1);
  });

  it("searches within built-in plugins and restores keyboard focus after Escape", async () => {
    await render();
    const input = container.querySelector<HTMLInputElement>('input')!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, "飞书");
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(container.querySelectorAll(".aw-plugin-card")).toHaveLength(1);
    const trigger = button("查看插件 飞书"); trigger.focus();
    await click("查看插件 飞书");
    await act(async () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })));
    expect(document.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(client.cancelCliAuth).not.toHaveBeenCalled();
  });

});
