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
    }))).toBe("authorize");
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
    expect(document.body.textContent).toContain("正在下载官方 Skills");
    const cancel = Array.from(document.querySelectorAll("button")).find((button) => button.textContent === "取消安装");
    await act(async () => cancel?.click());
    expect(onAction).toHaveBeenCalledWith("cancel_install");
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
    const connected = document.querySelector<HTMLButtonElement>(".cli-connector-detail-action-connected");
    expect(connected?.textContent).toContain("已连接");
    expect(connected?.textContent).toContain("取消登录");
    await act(async () => connected?.click());
    expect(onAction).toHaveBeenCalledWith("logout");
  });
});
