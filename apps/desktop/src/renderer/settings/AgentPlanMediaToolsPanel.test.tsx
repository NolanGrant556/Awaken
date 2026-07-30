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
  modelIds: ["glm-5.2", "doubao-seedream-test", "doubao-seedance-test"],
  catalog: [
    { id: "glm-5.2", kind: "llm" },
    { id: "doubao-seedream-test", kind: "image" },
    { id: "doubao-seedance-test", kind: "video" },
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

    const imageSelect = container.querySelector<HTMLSelectElement>(
      '[aria-label="图片生成模型"]',
    );
    const videoSelect = container.querySelector<HTMLSelectElement>(
      '[aria-label="视频生成模型"]',
    );
    expect(
      Array.from(imageSelect?.options ?? []).map((option) => option.value),
    ).toEqual(["", "doubao-seedream-test"]);
    expect(
      Array.from(videoSelect?.options ?? []).map((option) => option.value),
    ).toEqual(["", "doubao-seedance-test"]);

    await act(async () => {
      if (!imageSelect) return;
      imageSelect.value = "doubao-seedream-test";
      imageSelect.dispatchEvent(new Event("change", { bubbles: true }));
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

    const imageSelect = container.querySelector<HTMLSelectElement>(
      '[aria-label="图片生成模型"]',
    );
    expect(imageSelect?.value).toBe("doubao-seedream-test");
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
      container.querySelector<HTMLSelectElement>('[aria-label="图片生成模型"]')
        ?.disabled,
    ).toBe(true);
    expect(container.textContent).toContain("请先配置 Agent Plan 账户");
  });
});
