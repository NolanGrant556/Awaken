// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CliConnectorReadiness, RuntimeConnector } from "../shared/runtime";
import {
  CliConnectorDetailDialog,
  cliConnectorDialogAction,
} from "./CliConnectorDetailDialog";

const connector: RuntimeConnector = {
  id: "dingtalk-workspace",
  kind: "cli",
  displayName: "钉钉",
  description: "连接钉钉",
  category: "productivity",
  provider: "dingtalk-workspace",
  state: "needs-auth",
  sourceOrigin: "builtin",
  authType: "cli",
};

function readiness(overrides: Partial<CliConnectorReadiness> = {}): CliConnectorReadiness {
  return {
    providerId: "dingtalk-workspace",
    activation: "enabled",
    install: "missing",
    executable: "missing",
    configuration: "unknown",
    authentication: "unknown",
    overall: "not_ready",
    updatedAt: "2026-07-17T00:00:00.000Z",
    ...overrides,
  };
}

describe("CliConnectorDetailDialog", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("routes install, cancellation, authorization, retry and logout from the modal", () => {
    expect(cliConnectorDialogAction(readiness())).toBe("install");
    expect(cliConnectorDialogAction(readiness({ install: "installing", overall: "working" }))).toBe("cancel_install");
    expect(cliConnectorDialogAction(readiness({
      install: "installed",
      executable: "available",
      configuration: "ready",
      authentication: "required",
      overall: "action_required",
    }))).toBe("retry_authorization");
    expect(cliConnectorDialogAction(readiness({
      install: "installed",
      executable: "available",
      configuration: "ready",
      authentication: "in_progress",
      overall: "working",
    }), {
      id: "waiting-auth",
      provider: "dingtalk-workspace",
      phase: "waiting_user",
    })).toBe("cancel_authorization");
    expect(cliConnectorDialogAction(readiness({
      install: "installed",
      executable: "available",
      configuration: "ready",
      authentication: "required",
      overall: "action_required",
    }), {
      id: "failed-auth",
      provider: "dingtalk-workspace",
      phase: "failed",
    })).toBe("retry_authorization");
    expect(cliConnectorDialogAction(readiness({
      install: "installed",
      executable: "available",
      configuration: "ready",
      authentication: "authorized",
      overall: "ready",
    }))).toBe("logout");
  });

  it("lets the user cancel authorization while the external browser is waiting", async () => {
    const onAction = vi.fn();
    await act(async () => {
      root.render(
        <CliConnectorDetailDialog
          open
          connector={connector}
          readiness={readiness({
            install: "installed",
            executable: "available",
            configuration: "ready",
            authentication: "in_progress",
            overall: "working",
          })}
          authSession={{
            id: "waiting-auth",
            provider: "dingtalk-workspace",
            phase: "waiting_user",
          }}
          onOpenChange={vi.fn()}
          onAction={onAction}
        />,
      );
    });

    expect(document.body.textContent).toContain("请在已打开的官方网页完成授权");
    const cancel = Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
      .find((button) => button.textContent === "取消授权");
    expect(cancel?.disabled).toBe(false);
    await act(async () => cancel?.click());
    expect(onAction).toHaveBeenCalledWith("cancel_authorization");
  });

  it("cancels an active authorization session when the detail dialog closes", async () => {
    const onAction = vi.fn();
    const onOpenChange = vi.fn();
    await act(async () => {
      root.render(
        <CliConnectorDetailDialog
          open
          connector={connector}
          readiness={readiness({
            install: "installed",
            executable: "available",
            configuration: "ready",
            authentication: "in_progress",
            overall: "working",
          })}
          authSession={{
            id: "waiting-auth",
            provider: "dingtalk-workspace",
            phase: "waiting_user",
          }}
          onOpenChange={onOpenChange}
          onAction={onAction}
        />,
      );
    });

    await act(async () => {
      document.querySelector<HTMLButtonElement>("button[aria-label='关闭']")?.click();
    });
    expect(onAction).toHaveBeenCalledOnce();
    expect(onAction).toHaveBeenCalledWith("cancel_authorization");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("keeps an active installation running when the detail dialog closes", async () => {
    const onAction = vi.fn();
    const onOpenChange = vi.fn();
    await act(async () => {
      root.render(
        <CliConnectorDetailDialog
          open
          connector={connector}
          readiness={readiness({ install: "installing", overall: "working" })}
          installProgress={{ phase: "downloading_cli", percent: 42, label: "正在下载官方 CLI" }}
          onOpenChange={onOpenChange}
          onAction={onAction}
        />,
      );
    });

    await act(async () => {
      document.querySelector<HTMLButtonElement>("button[aria-label='关闭']")?.click();
    });
    expect(onAction).not.toHaveBeenCalled();
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("renders an installed but logged-out connector as reauthorization without an install failure", async () => {
    const onAction = vi.fn();
    await act(async () => {
      root.render(
        <CliConnectorDetailDialog
          open
          connector={connector}
          readiness={readiness({
            install: "installed",
            executable: "available",
            configuration: "ready",
            authentication: "required",
            overall: "action_required",
          })}
          onOpenChange={vi.fn()}
          onAction={onAction}
        />,
      );
    });

    expect(document.body.textContent).toContain("运行组件与官方技能已安装，当前未授权");
    expect(document.body.textContent).not.toContain("CLI 操作失败");
    expect(document.querySelector(".aw-product-logo")).not.toBeNull();
    const reauthorize = Array.from(document.querySelectorAll("button"))
      .find((button) => button.textContent === "重新授权");
    expect(reauthorize).toBeTruthy();
    await act(async () => reauthorize?.click());
    expect(onAction).toHaveBeenCalledWith("retry_authorization");
  });

  it("shows live progress and sends cancel installation from the footer button", async () => {
    const onAction = vi.fn();
    await act(async () => {
      root.render(
        <CliConnectorDetailDialog
          open
          connector={connector}
          readiness={readiness({ install: "installing", overall: "working" })}
          installProgress={{ phase: "downloading_skills", percent: 72, label: "正在下载官方 Skills" }}
          onOpenChange={vi.fn()}
          onAction={onAction}
        />,
      );
    });
    const progress = document.querySelector<HTMLElement>("[role='progressbar']");
    expect(progress?.getAttribute("aria-valuenow")).toBe("72");
    expect(document.body.textContent).toContain("正在下载官方 技能");
    const cancel = Array.from(document.querySelectorAll("button")).find((button) => button.textContent === "取消安装");
    await act(async () => cancel?.click());
    expect(onAction).toHaveBeenCalledWith("cancel_install");
  });

  it("keeps the close control accessible without rendering a close tooltip", async () => {
    await act(async () => {
      root.render(
        <CliConnectorDetailDialog
          open
          connector={connector}
          readiness={readiness()}
          onOpenChange={vi.fn()}
          onAction={vi.fn()}
        />,
      );
    });

    const close = document.querySelector<HTMLButtonElement>("button[aria-label='关闭']");
    expect(close).toBeTruthy();
    expect(close?.getAttribute("title")).toBeNull();
    expect(close?.closest(".aw-dialog__head--title-visually-hidden")).toBeTruthy();
    expect(document.querySelector("[role='tooltip']")).toBeNull();
  });

  it("renders the connected control green-ready and exposes real logout on click", async () => {
    const onAction = vi.fn();
    await act(async () => {
      root.render(
        <CliConnectorDetailDialog
          open
          connector={connector}
          readiness={readiness({
            install: "installed",
            executable: "available",
            configuration: "ready",
            authentication: "authorized",
            overall: "ready",
            account: { displayName: "Nolan", organization: "Awaken" },
          })}
          onOpenChange={vi.fn()}
          onAction={onAction}
        />,
      );
    });
    const connected = document.querySelector<HTMLButtonElement>("[data-action='logout']");
    expect(connected?.textContent).toContain("已连接");
    expect(connected?.textContent).toContain("取消登录");
    await act(async () => connected?.click());
    expect(onAction).toHaveBeenCalledWith("logout");
  });
});
