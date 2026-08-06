// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AgentPlanAccount,
  AgentPlanMediaToolConfiguration,
  AgentPlanMediaTools,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { AgentPlanMediaToolsPanel } from "./AgentPlanMediaToolsPanel";

const account: AgentPlanAccount = {
  accountId: "volcengine-agent-plan-default",
  providerId: "volcengine-agent-plan",
  hasKey: true,
  maskedKey: "•••• 1234",
  credentialRevision: 1,
  modelIds: [
    "glm-5.2",
    "doubao-seedream-test",
    "doubao-seedance-2.0",
    "doubao-seedance-2.0-fast",
    "doubao-seedance-2.0-mini",
    "doubao-seedance-1.5-pro",
  ],
  catalog: [
    { id: "glm-5.2", kind: "llm" },
    { id: "doubao-seedream-test", kind: "image" },
    { id: "doubao-seedance-2.0", kind: "video" },
    { id: "doubao-seedance-2.0-fast", kind: "video" },
    { id: "doubao-seedance-2.0-mini", kind: "video" },
    { id: "doubao-seedance-1.5-pro", kind: "video" },
  ],
  catalogUpdatedAt: "2026-07-30T00:00:00.000Z",
};

const emptySettings: AgentPlanMediaTools = { configurations: [] };

const imageConfiguration: AgentPlanMediaToolConfiguration = {
  id: "volcengine-agent-plan-image",
  providerId: "volcengine-agent-plan",
  accountId: "volcengine-agent-plan-default",
  capability: "media.image.generate",
  modelId: "doubao-seedream-test",
  modelKind: "image",
  enabled: true,
  configurationRevision: 1,
  credentialRevision: 1,
  displayState: "ready_unverified",
};

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("AgentPlanMediaToolsPanel", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 768,
    });
    vi.restoreAllMocks();
  });

  it("shows compact image and video cards without another API key form", async () => {
    const saveAgentPlanMediaTool = vi.fn(async () => ({
      settings: { configurations: [imageConfiguration] },
      restartStatus: "restarted" as const,
    }));
    const runtimeClient = {
      getAgentPlanMediaTools: vi.fn(async () => emptySettings),
      saveAgentPlanMediaTool,
    } as unknown as RuntimeClient;
    const toast = vi.fn();

    await act(async () => {
      root.render(
        <AgentPlanMediaToolsPanel
          account={account}
          runtimeClient={runtimeClient}
          onConfigureAccount={vi.fn()}
          onSettingsChanged={vi.fn()}
          onToast={toast}
        />,
      );
    });
    await settle();

    expect(container.textContent).toContain("图片与视频工具");
    expect(container.textContent).toContain("火山 Agent Plan：图片生成");
    expect(container.textContent).toContain("火山 Agent Plan：视频生成");
    expect(container.textContent).not.toContain("API Key");
    expect(container.querySelector(".model-provider-card")).toBeNull();

    const imageSelect = container.querySelector<HTMLButtonElement>(
      '[aria-label="图片生成模型"]',
    );
    const videoSelect = container.querySelector<HTMLButtonElement>(
      '[aria-label="视频生成模型"]',
    );
    expect(imageSelect?.tagName).toBe("BUTTON");
    expect(videoSelect?.tagName).toBe("BUTTON");
    const triggerRectSpy = vi
      .spyOn(videoSelect!, "getBoundingClientRect")
      .mockReturnValue({
        top: 430,
        bottom: 470,
        left: 100,
        right: 700,
        width: 600,
        height: 40,
        x: 100,
        y: 430,
        toJSON: () => ({}),
      });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 500,
    });

    await act(async () => videoSelect?.click());
    const videoSelectRoot = videoSelect?.closest(".aw-select");
    expect(videoSelectRoot?.getAttribute("data-menu-placement")).toBe("top");
    const videoList = videoSelect
      ?.closest("article")
      ?.querySelector('[role="listbox"]');
    const videoOptions = Array.from(
      videoList?.querySelectorAll<HTMLButtonElement>('[role="option"]') ?? [],
    );
    expect(videoOptions.map((option) => option.dataset.value)).toEqual([
      "doubao-seedance-2.0",
      "doubao-seedance-2.0-fast",
      "doubao-seedance-2.0-mini",
      "doubao-seedance-1.5-pro",
    ]);
    await act(async () => {
      videoOptions[0]?.focus();
      videoOptions[0]?.dispatchEvent(
        new KeyboardEvent("keydown", { key: "End", bubbles: true }),
      );
    });
    expect(document.activeElement).toBe(videoOptions.at(-1));

    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 900,
    });
    await act(async () => window.dispatchEvent(new Event("resize")));
    expect(videoSelectRoot?.getAttribute("data-menu-placement")).toBe(
      "bottom",
    );

    triggerRectSpy.mockReturnValue({
      top: 110,
      bottom: 150,
      left: 100,
      right: 700,
      width: 600,
      height: 40,
      x: 100,
      y: 110,
      toJSON: () => ({}),
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 260,
    });
    await act(async () => window.dispatchEvent(new Event("resize")));
    expect(videoList?.getAttribute("style")).toContain("max-height: 96px");
    await act(async () => videoSelect?.click());

    await act(async () => imageSelect?.click());
    const imageList = imageSelect
      ?.closest("article")
      ?.querySelector('[role="listbox"]');
    expect(
      Array.from(imageList?.querySelectorAll('[role="option"]') ?? []).map(
        (option) => option.getAttribute("data-value"),
      ),
    ).toEqual(["doubao-seedream-test"]);

    await act(async () => {
      imageList
        ?.querySelector<HTMLButtonElement>(
          '[role="option"][data-value="doubao-seedream-test"]',
        )
        ?.click();
    });
    const imageCard = imageSelect?.closest("article");
    const saveButton = Array.from(
      imageCard?.querySelectorAll<HTMLButtonElement>("button") ?? [],
    ).find((button) => button.textContent?.trim() === "保存");
    await act(async () => saveButton?.click());

    expect(saveAgentPlanMediaTool).toHaveBeenCalledWith({
      capability: "media.image.generate",
      modelId: "doubao-seedream-test",
      enabled: true,
    });
    expect(toast).toHaveBeenCalledWith("图片生成工具已保存");
  });

  it("keeps configured tools on the same page with switch and edit actions", async () => {
    const runtimeClient = {
      getAgentPlanMediaTools: vi.fn(async () => ({
        configurations: [imageConfiguration],
      })),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <AgentPlanMediaToolsPanel
          account={account}
          runtimeClient={runtimeClient}
          onConfigureAccount={vi.fn()}
          onSettingsChanged={vi.fn()}
          onToast={vi.fn()}
        />,
      );
    });
    await settle();

    const imageSelect = container.querySelector<HTMLButtonElement>(
      '[aria-label="图片生成模型"]',
    );
    expect(imageSelect?.textContent).toContain("doubao-seedream-test");
    expect(container.textContent).toContain("保存更改");
    expect(container.textContent).toContain("删除");
    expect(container.textContent).not.toContain("已配置工具");
    expect(
      container.querySelector('[role="switch"][aria-label*="图片生成"]'),
    ).not.toBeNull();
  });

  it("routes an unconfigured account back to account setup", async () => {
    const onConfigureAccount = vi.fn();
    const runtimeClient = {
      getAgentPlanMediaTools: vi.fn(async () => emptySettings),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <AgentPlanMediaToolsPanel
          account={{ ...account, hasKey: false, maskedKey: undefined }}
          runtimeClient={runtimeClient}
          onConfigureAccount={onConfigureAccount}
          onSettingsChanged={vi.fn()}
          onToast={vi.fn()}
        />,
      );
    });
    await settle();

    const configureButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.trim() === "配置账户");
    await act(async () => configureButton?.click());

    expect(onConfigureAccount).toHaveBeenCalledOnce();
    expect(
      container.querySelector<HTMLButtonElement>('[aria-label="图片生成模型"]')
        ?.disabled,
    ).toBe(true);
    expect(container.textContent).toContain("请先配置 Agent Plan 账户");
  });
});
