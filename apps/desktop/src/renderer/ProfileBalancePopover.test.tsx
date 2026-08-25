// @vitest-environment jsdom

import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ProfileBalancePopover,
  credentialRecoveryForBalances,
} from "./ProfileBalancePopover";
import type { RuntimeClient } from "./runtime/client";
import type { ModelProfileBalance } from "../shared/runtime";

const mockBalances: ModelProfileBalance[] = [
  {
    profileId: "kimi-coding-plan",
    providerId: "kimi-coding-plan",
    modelId: "k3",
    displayName: "Kimi Coding Plan",
    status: "available",
    fetchedAt: "2026-01-01T00:00:00.000Z",
    detail: {
      kind: "quota",
      quotas: [{ name: "本周请求", limit: 100, used: 99, remaining: 1 }],
    },
  },
  {
    profileId: "volcengine-agent-plan",
    providerId: "volcengine-agent-plan",
    modelId: "ark-code-latest",
    displayName: "火山 Agent Plan",
    status: "requires_setup",
    error: "火山 Agent Plan 推理 Key 不能查询套餐用量；GetAFPUsage 需要独立的火山云 AK/SK 与只读方舟管理权限。",
    setupURL: "https://api.volcengine.com/api-docs/view?action=GetAFPUsage&serviceCode=ark&version=2024-01-01",
    setupLabel: "查看 Agent Plan 用量授权",
  },
  {
    profileId: "minimax-token-plan",
    providerId: "minimax-token-plan",
    modelId: "MiniMax-M3",
    displayName: "MiniMax Token Plan",
    status: "available",
    detail: {
      kind: "quota",
      quotas: [
        { name: "5 小时额度", limit: 100, used: 0, remaining: 100, unit: "percent" },
        { name: "周额度", limit: 0, used: 0, remaining: 0, unlimited: true },
      ],
    },
  },
];

function buttonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.includes(text),
  );
}

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("ProfileBalancePopover", () => {
  let container: HTMLDivElement;
  let anchor: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    anchor = document.createElement("div");
    document.body.append(anchor, container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("renders nothing when the personal popover and usage dialog are closed", async () => {
    await act(async () => {
      root.render(
        <ProfileBalancePopover
          open={false}
          anchorRef={{ current: anchor }}
          runtimeClient={{} as RuntimeClient}
          themePreference="light"
          themeSaving={false}
          onThemePreferenceChange={() => {}}
          onOpenSettings={() => {}}
          onClose={() => {}}
        />,
      );
    });
    expect(document.querySelector(".profile-balance-popover")).toBeNull();
    expect(document.querySelector(".aw-profile-usage-dialog")).toBeNull();
  });

  it("matches the account button width and inset position", async () => {
    vi.spyOn(anchor, "getBoundingClientRect").mockReturnValue({
      x: 14,
      y: 700,
      left: 14,
      top: 700,
      right: 242,
      bottom: 744,
      width: 228,
      height: 44,
      toJSON: () => ({}),
    });

    await act(async () => {
      root.render(
        <ProfileBalancePopover
          open
          anchorRef={{ current: anchor }}
          runtimeClient={{} as RuntimeClient}
          themePreference="light"
          themeSaving={false}
          onThemePreferenceChange={() => {}}
          onOpenSettings={() => {}}
          onClose={() => {}}
        />,
      );
    });

    const popover = document.querySelector<HTMLElement>(".profile-balance-popover");
    const panel = document.querySelector<HTMLElement>("#aw-profile-menu-panel");
    expect(popover?.style.left).toBe("14px");
    expect(popover?.style.width).toBe("228px");
    expect(panel?.style.width).toBe("100%");
  });

  it("keeps only the usage entry in the personal popover and defers querying until click", async () => {
    const getModelProfileBalances = vi.fn(async () => ({ balances: mockBalances }));
    const onClose = vi.fn();
    const onThemePreferenceChange = vi.fn();

    await act(async () => {
      root.render(
        <ProfileBalancePopover
          open
          anchorRef={{ current: anchor }}
          runtimeClient={{ getModelProfileBalances } as unknown as RuntimeClient}
          themePreference="system"
          themeSaving={false}
          onThemePreferenceChange={onThemePreferenceChange}
          onOpenSettings={() => {}}
          onClose={onClose}
        />,
      );
    });

    expect(buttonByText("查询用量")).toBeDefined();
    const appearanceButton = buttonByText("外观");
    const usageButton = buttonByText("查询用量");
    const settingsButton = buttonByText("设置");
    expect(appearanceButton?.classList.contains("aw-profile-menu-panel__action")).toBe(true);
    expect(usageButton?.classList.contains("aw-profile-menu-panel__action")).toBe(true);
    expect(settingsButton?.classList.contains("aw-profile-menu-panel__action")).toBe(true);
    expect(usageButton?.querySelectorAll("svg")).toHaveLength(1);
    expect(appearanceButton?.compareDocumentPosition(usageButton as Node) ?? 0).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(usageButton?.compareDocumentPosition(settingsButton as Node) ?? 0).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(document.querySelector('[role="radiogroup"][aria-label="主题"]')).toBeNull();
    expect(document.querySelector(".profile-balance-popover")?.textContent).not.toContain("剩余用量");
    expect(getModelProfileBalances).not.toHaveBeenCalled();
    expect(document.querySelector(".profile-balance-popover")?.textContent).toContain("设置");

    await act(async () => appearanceButton?.click());
    expect(appearanceButton?.getAttribute("aria-expanded")).toBe("true");
    expect(document.querySelector('[role="dialog"][aria-label="外观"]')).not.toBeNull();
    expect(document.querySelector('[role="radiogroup"][aria-label="主题"]')).not.toBeNull();
    expect(document.querySelector(".profile-balance-popover")?.textContent).toContain("跟随系统");
    expect(getModelProfileBalances).not.toHaveBeenCalled();

    await act(async () => buttonByText("浅色")?.click());
    expect(onThemePreferenceChange).toHaveBeenCalledWith("light");
    expect(appearanceButton?.getAttribute("aria-expanded")).toBe("false");
    expect(document.querySelector('[role="dialog"][aria-label="外观"]')).toBeNull();

    await act(async () => buttonByText("查询用量")?.click());
    await settle();

    expect(onClose).toHaveBeenCalledTimes(1);
    expect(getModelProfileBalances).toHaveBeenCalledTimes(1);
    expect(document.querySelector(".aw-profile-usage-dialog")?.textContent).toContain("用量");
    expect(document.querySelector(".aw-profile-balance-panel__nav")?.textContent).toContain("API / Plan");
    expect(document.querySelector(".aw-profile-balance-panel__detail")?.textContent).toContain("99%");
    expect(document.querySelector(".aw-profile-balance-panel__detail-label")).toBeNull();
    expect(document.querySelector('[data-provider-brand="kimi"]')).not.toBeNull();
    expect(document.querySelector(".aw-profile-usage-dialog")?.textContent).not.toContain(
      "k3",
    );
    expect(document.querySelector(".aw-profile-usage-dialog")?.textContent).not.toContain(
      "ark-code-latest",
    );

    await act(async () => buttonByText("MiniMax Token Plan")?.click());
    expect(document.querySelector(".aw-profile-balance-panel__detail")?.textContent).toContain(
      "5 小时额度",
    );
    expect(document.querySelector(".aw-profile-balance-panel__detail")?.textContent).toContain(
      "周额度∞无限制",
    );
    expect(document.querySelector(".aw-profile-balance-panel__detail")?.textContent).not.toContain(
      "video",
    );

    await act(async () => buttonByText("火山 Agent Plan")?.click());
    expect(document.querySelector('[data-provider-brand="volcengine"]')).not.toBeNull();
    expect(document.querySelector(".aw-profile-balance-panel__detail")?.textContent).toContain(
      "需要独立的火山云 AK/SK",
    );
    const setupLink = document.querySelector<HTMLAnchorElement>(
      ".aw-profile-balance-panel__setup-link",
    );
    expect(setupLink?.textContent).toBe("查看 Agent Plan 用量授权");
    expect(setupLink?.href).toContain("GetAFPUsage");
    expect(document.querySelector(".aw-profile-balance-panel__local-usage")).toBeNull();
  });

  it("collapses repeated provider failures into one credential recovery state", () => {
    expect(
      credentialRecoveryForBalances([
        { ...mockBalances[0], status: "error", credentialIssue: "unreadable" },
        {
          ...mockBalances[0],
          profileId: "deepseek",
          providerId: "deepseek",
          credentialIssue: "unreadable",
        },
      ]),
    ).toBe("unreadable");
  });
});
