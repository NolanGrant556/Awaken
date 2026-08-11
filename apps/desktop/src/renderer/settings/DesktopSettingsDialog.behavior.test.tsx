// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  ModelProfileTestResult,
  ModelProvider,
  RuntimeModelProfileSummary,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import { DesktopSettingsDialog } from "./DesktopSettingsDialog";

const deepSeekProvider: ModelProvider = {
  id: "deepseek",
  displayName: "DeepSeek",
  baseURL: "https://api.deepseek.com",
  protocol: "openai_chat",
  supportedProtocols: ["openai_chat", "openai_responses", "anthropic_messages"],
  officialProtocols: ["openai_chat"],
  protocolBaseURLs: {
    openai_chat: "https://api.deepseek.com",
    openai_responses: "https://api.deepseek.com",
    anthropic_messages: "https://api.deepseek.com/anthropic",
  },
  protocolAuthSchemes: {},
  catalogStrategy: "openai",
  catalogSource: "official_manifest",
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
  supportedProtocols: ["openai_chat", "openai_responses", "anthropic_messages"],
  officialProtocols: ["openai_chat"],
  protocolBaseURLs: {},
  protocolAuthSchemes: {},
  catalogStrategy: "openai",
  catalogSource: "official_manifest",
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

function providerButton(name = "DeepSeek"): HTMLButtonElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLButtonElement>("button"),
  ).find(
    (button) =>
      button.classList.contains("model-provider-card") &&
      button.textContent?.includes(name),
  );
}

function accessTypeButton(name: string): HTMLButtonElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLButtonElement>(".model-settings-tab"),
  ).find((button) => button.textContent?.trim() === name);
}

function providerModalSelectTrigger(label: string): HTMLButtonElement {
  const trigger = Array.from(
    document.querySelectorAll<HTMLButtonElement>(".aw-select__trigger"),
  ).find((button) => {
    const labelId = button.getAttribute("aria-labelledby");
    return labelId ? document.getElementById(labelId)?.textContent === label : false;
  });
  if (!trigger) {
    throw new Error(`Select trigger not found for ${label}`);
  }
  return trigger;
}

function testButton(): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(
    ".model-provider-primary-test",
  );
}

function enterApiKey(value = "test-key"): void {
  const input = document.querySelector<HTMLInputElement>(
    ".model-provider-secret-field input",
  );
  if (!input) throw new Error("API key input not found");
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function enterTextInput(
  input: HTMLInputElement,
  value: string,
): void {
  const setter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
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
    expect(testButton()?.disabled).toBe(true);
    await act(async () => enterApiKey());
    const button = testButton();
    expect(button).not.toBeNull();
    expect(button?.disabled).toBe(false);

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
    await act(async () => enterApiKey());
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

  it("shows the three LLM protocols in the provider modal and refreshes the catalog status", async () => {
    const refreshResult = deferred<{ provider: ModelProvider; message: string }>();
    const runtimeClient = {
      listModelProviders: vi.fn(async () => [deepSeekProvider]),
      listModelProfiles: vi.fn(async () => []),
      getAgentPlanAccount: vi.fn(async () => null),
      syncModelCapabilities: vi.fn(async () => [deepSeekProvider]),
      refreshModelCatalog: vi.fn(async () => refreshResult.promise),
      testModelProfile: vi.fn(),
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
          initialModelSection="api"
        />,
      );
    });
    await settle();

    await act(async () => providerButton()?.click());
    await settle();

    const baseURLInput = Array.from(
      document.querySelectorAll<HTMLInputElement>("input"),
    ).find((input) => input.value === "https://api.deepseek.com");
    expect(baseURLInput).toBeTruthy();
    expect(baseURLInput?.readOnly).toBe(true);

    const protocolTrigger = providerModalSelectTrigger("接口协议");
    await act(async () => protocolTrigger.click());

    const protocolOptions = Array.from(
      document.querySelectorAll<HTMLButtonElement>(".aw-select__option"),
    ).map((button) => button.getAttribute("data-value"));
    expect(protocolOptions).toEqual([
      "openai_chat",
      "openai_responses",
      "anthropic_messages",
    ]);

    const messagesOption = Array.from(
      document.querySelectorAll<HTMLButtonElement>(".aw-select__option"),
    ).find((button) => button.getAttribute("data-value") === "anthropic_messages");
    await act(async () => messagesOption?.click());
    expect(baseURLInput?.value).toBe("https://api.deepseek.com/anthropic");
    await act(async () => enterApiKey());

    const refreshButton = Array.from(
      document.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("刷新模型目录"));
    expect(refreshButton).toBeTruthy();

    await act(async () => refreshButton?.click());
    expect(refreshButton?.disabled).toBe(true);
    expect(container.textContent).not.toContain("当前先展示官方清单与本地缓存；填写或复用 API Key 后可从供应商 API 刷新。");

    await act(async () =>
      refreshResult.resolve({
        provider: {
          ...deepSeekProvider,
          models: ["deepseek-v4-pro", "deepseek-v4-ultra"],
          catalogSource: "provider_api",
          catalogObservedAt: "2026-08-10T00:00:00.000Z",
        },
        message: "已从 DeepSeek API 同步 2 个模型。",
      }),
    );

    expect(container.textContent).toContain("已从 DeepSeek API 同步 2 个模型。");
    expect(refreshButton?.disabled).toBe(false);
    expect(runtimeClient.refreshModelCatalog).toHaveBeenCalledTimes(1);
  });

  it("labels a custom endpoint as user-declared instead of an official three-protocol provider", async () => {
    const customProvider: ModelProvider = {
      ...deepSeekProvider,
      id: "custom",
      displayName: "自定义 API",
      baseURL: "",
      officialProtocols: [],
      defaultModel: "",
      models: [],
      docsURL: "",
      apiDocsURL: "",
      allowCustomModel: true,
      catalogStrategy: "none",
      accessType: "custom",
    };
    const runtimeClient = {
      listModelProviders: vi.fn(async () => [deepSeekProvider, customProvider]),
      listModelProfiles: vi.fn(async () => []),
      getAgentPlanAccount: vi.fn(async () => null),
      syncModelCapabilities: vi.fn(async () => [deepSeekProvider, customProvider]),
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
          initialModelSection="api"
        />,
      );
    });
    await settle();
    expect(providerButton("DeepSeek")).toBeTruthy();
    expect(providerButton("自定义 API")).toBeUndefined();

    await act(async () => accessTypeButton("自定义")?.click());
    expect(providerButton("DeepSeek")).toBeUndefined();
    expect(providerButton("自定义 API")).toBeTruthy();

    await act(async () => providerButton("自定义 API")?.click());
    const protocolTrigger = providerModalSelectTrigger("接口协议");
    await act(async () => protocolTrigger.click());

    const labels = Array.from(
      document.querySelectorAll<HTMLButtonElement>(".aw-select__option"),
    ).map((button) => button.textContent);
    expect(labels).toEqual([
      "OpenAI Chat Completions",
      "OpenAI Responses",
      "Anthropic Messages",
    ]);
    expect(container.textContent).toContain("选择接口协议并填写对应的 Base URL");
    expect(document.querySelector('input[placeholder="输入模型 ID"]')).toBeTruthy();
    expect(document.querySelector(".model-custom-picker-trigger")).toBeNull();
    expect(
      Array.from(document.querySelectorAll("button")).some((button) =>
        button.textContent?.includes("刷新模型目录"),
      ),
    ).toBe(false);
  });

  it("explains a static catalog instead of presenting a refresh action that cannot work", async () => {
    const staticProvider: ModelProvider = {
      ...deepSeekProvider,
      id: "kimi-coding-plan",
      displayName: "Kimi Coding Plan",
      catalogStrategy: "none",
      accessType: "coding_plan",
      models: ["k3"],
      defaultModel: "k3",
    };
    const runtimeClient = {
      listModelProviders: vi.fn(async () => [staticProvider]),
      listModelProfiles: vi.fn(async () => []),
      getAgentPlanAccount: vi.fn(async () => null),
      syncModelCapabilities: vi.fn(async () => [staticProvider]),
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
          initialModelSection="coding_plan"
        />,
      );
    });
    await settle();
    await act(async () => providerButton("Kimi Coding Plan")?.click());

    expect(container.textContent).toContain("未提供可由当前推理凭据调用的模型目录 API");
    expect(
      Array.from(document.querySelectorAll("button")).some((button) =>
        button.textContent?.includes("刷新模型目录"),
      ),
    ).toBe(false);
  });

  it("lets users hand-enter an unknown context window and preserves the saved override", async () => {
    let profiles: RuntimeModelProfileSummary[] = [
      {
        id: "profile-deepseek",
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        hasKey: true,
        enabled: true,
        isDefault: true,
        profileStatus: "ready",
        advanced: {
          maxTokens: 8192,
        },
      },
    ];
    const runtimeClient = {
      listModelProviders: vi.fn(async () => [deepSeekProvider]),
      listModelProfiles: vi.fn(async () => profiles),
      getAgentPlanAccount: vi.fn(async () => null),
      syncModelCapabilities: vi.fn(async () => [deepSeekProvider]),
      saveModelProfile: vi.fn(async (request) => {
        profiles = profiles.map((profile) =>
          profile.id === request.id
            ? {
                ...profile,
                advanced: {
                  ...profile.advanced,
                  ...request.advanced,
                },
              }
            : profile,
        );
        return profiles[0];
      }),
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
          initialModelSection="api"
        />,
      );
    });
    await settle();

    const llmButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.trim() === "LLM 设置");
    await act(async () => llmButton?.click());
    await settle();

    expect(container.textContent).not.toContain("未知（等待目录数据或用户覆盖）");
    expect(container.textContent).toContain("当前目录没有返回上下文窗口，可手动输入后保存。");
    expect(container.querySelector(".configured-context-range")).toBeNull();

    const contextInput = container.querySelector<HTMLInputElement>(
      ".configured-context-input",
    );
    expect(contextInput?.value).toBe("");

    await act(async () => {
      if (contextInput) enterTextInput(contextInput, "64000");
    });

    const saveButton = Array.from(
      container.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) => button.textContent?.includes("保存 LLM 设置"));
    expect(saveButton?.disabled).toBe(false);

    await act(async () => saveButton?.click());
    await settle();

    expect(runtimeClient.saveModelProfile).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "profile-deepseek",
        advanced: expect.objectContaining({
          contextWindow: 64000,
          maxTokens: 8192,
        }),
      }),
    );

    const reloadedInput = container.querySelector<HTMLInputElement>(
      ".configured-context-input",
    );
    expect(reloadedInput?.value).toBe("64000");
  });
});
