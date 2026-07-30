// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  ModelProfileTestResult,
  ModelProvider,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { DesktopSettingsDialog } from "./DesktopSettingsDialog";

const deepSeekProvider: ModelProvider = {
  id: "deepseek",
  displayName: "DeepSeek",
  baseURL: "https://api.deepseek.com",
  protocol: "openai_chat",
  authScheme: "bearer",
  keyEnvName: "DEEPSEEK_API_KEY",
  defaultModel: "deepseek-v4-pro",
  models: ["deepseek-v4-pro"],
  docsURL: "https://api-docs.deepseek.com",
  apiDocsURL: "https://platform.deepseek.com/api_keys",
  accessType: "api",
  codingPreset: false,
  endpointModel: false,
  allowCustomModel: false,
  modelCapabilities: {},
};

const agentPlanProvider: ModelProvider = {
  id: "volcengine-agent-plan",
  displayName: "火山 Agent Plan",
  baseURL: "https://ark.cn-beijing.volces.com/api/v3",
  protocol: "openai_chat",
  authScheme: "bearer",
  keyEnvName: "ARK_API_KEY",
  defaultModel: "ark-code-latest",
  models: ["ark-code-latest"],
  docsURL: "https://www.volcengine.com/docs/82379",
  apiDocsURL: "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey",
  accessType: "agent_plan",
  codingPreset: false,
  endpointModel: false,
  allowCustomModel: false,
  modelCapabilities: {},
};

function runtimeClientWithTest(
  testModelProfile: RuntimeClient["testModelProfile"],
): RuntimeClient {
  return {
    listModelProviders: vi.fn(async () => [deepSeekProvider]),
    listModelProfiles: vi.fn(async () => []),
    getAgentPlanAccount: vi.fn(async () => null),
    syncModelCapabilities: vi.fn(async () => [deepSeekProvider]),
    testModelProfile,
  } as unknown as RuntimeClient;
}

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function providerButton(): HTMLButtonElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLButtonElement>("button"),
  ).find(
    (button) =>
      button.classList.contains("model-provider-card") &&
      button.textContent?.includes("DeepSeek"),
  );
}

function testButton(): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(
    ".model-provider-primary-test",
  );
}

describe("DesktopSettingsDialog model connection feedback", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("keeps progress and the successful result visible inside the provider modal", async () => {
    let resolveTest: ((result: ModelProfileTestResult) => void) | undefined;
    const testModelProfile = vi.fn(
      () =>
        new Promise<ModelProfileTestResult>((resolve) => {
          resolveTest = resolve;
        }),
    );

    await act(async () => {
      root.render(
        <DesktopSettingsDialog
          open
          runtimeClient={runtimeClientWithTest(testModelProfile)}
          theme="light"
          themePreference="system"
          themeSaving={false}
          themeMessage={null}
          onThemePreferenceChange={vi.fn()}
          onOpenChange={vi.fn()}
          onProfilesChanged={vi.fn()}
          initialModelSection="api"
        />,
      );
    });
    await settle();

    await act(async () => providerButton()?.click());
    const button = testButton();
    expect(button).not.toBeNull();

    await act(async () => button?.click());
    expect(button?.disabled).toBe(true);
    expect(document.querySelector(".model-provider-test-feedback")).toBeNull();

    await act(async () => {
      resolveTest?.({
        ok: true,
        message: "连接测试通过：服务已响应 chat/completions。",
      });
    });

    const feedback = document.querySelector(".model-provider-test-feedback");
    expect(feedback?.textContent).toContain("连接测试通过");
    expect(
      feedback?.classList.contains("model-provider-test-feedback--success"),
    ).toBe(true);
    expect(feedback?.getAttribute("role")).toBe("status");
    expect(testButton()?.disabled).toBe(false);
  });

  it("shows a failed result in the same modal instead of a hidden global toast", async () => {
    const testModelProfile = vi.fn(async () => ({
      ok: false,
      message: "连接测试失败：API key 无效或未授权。",
    }));

    await act(async () => {
      root.render(
        <DesktopSettingsDialog
          open
          runtimeClient={runtimeClientWithTest(testModelProfile)}
          theme="dark"
          themePreference="dark"
          themeSaving={false}
          themeMessage={null}
          onThemePreferenceChange={vi.fn()}
          onOpenChange={vi.fn()}
          onProfilesChanged={vi.fn()}
          initialModelSection="api"
        />,
      );
    });
    await settle();

    await act(async () => providerButton()?.click());
    await act(async () => testButton()?.click());

    const feedback = document.querySelector(".model-provider-test-feedback");
    expect(feedback?.textContent).toContain("连接测试失败");
    expect(
      feedback?.classList.contains("model-provider-test-feedback--error"),
    ).toBe(true);
    expect(feedback?.getAttribute("role")).toBe("alert");
    expect(document.querySelector(".aw-settings-toast")).toBeNull();
  });

  it("keeps image and video tools inside the Agent Plan model page", async () => {
    const runtimeClient = {
      listModelProviders: vi.fn(async () => [agentPlanProvider]),
      listModelProfiles: vi.fn(async () => []),
      getAgentPlanAccount: vi.fn(async () => ({
        accountId: "volcengine-agent-plan-default",
        providerId: "volcengine-agent-plan",
        hasKey: true,
        credentialRevision: 1,
        modelIds: [
          "ark-code-latest",
          "doubao-seedream-test",
          "doubao-seedance-test",
        ],
        catalog: [
          { id: "ark-code-latest", kind: "llm" as const },
          { id: "doubao-seedream-test", kind: "image" as const },
          { id: "doubao-seedance-test", kind: "video" as const },
        ],
      })),
      getAgentPlanMediaTools: vi.fn(async () => ({ configurations: [] })),
      syncModelCapabilities: vi.fn(async () => [agentPlanProvider]),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(
        <DesktopSettingsDialog
          open
          runtimeClient={runtimeClient}
          theme="light"
          themePreference="system"
          themeSaving={false}
          themeMessage={null}
          onThemePreferenceChange={vi.fn()}
          onOpenChange={vi.fn()}
          onProfilesChanged={vi.fn()}
          initialModelSection="agent_plan"
        />,
      );
    });
    await settle();

    expect(container.textContent).not.toContain("图片视频工具");
    const modelToolsButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.trim() === "模型与工具");
    await act(async () => modelToolsButton?.click());
    await settle();

    expect(container.textContent).toContain("图片与视频工具");
    expect(container.textContent).toContain("火山 Agent Plan：图片生成");
    expect(container.textContent).toContain("火山 Agent Plan：视频生成");
    expect(container.textContent).not.toContain("已配置工具");
  });
});
