import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
  AwakenConfiguredModels,
  AwakenModelProviderForm,
  AwakenSettingsProviderCard,
  AwakenSettingsProviderModal,
} from "@awaken/design-system";
import type {
  ModelProvider,
  RuntimeModelProfileSummary,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import {
  AppearanceSettings,
  DesktopSettingsDialog,
  draftFromExistingProfile,
  modelCandidatesForProtocol,
  toSaveRequest,
} from "./DesktopSettingsDialog";

describe("DesktopSettingsDialog", () => {
  it("renders the shared full-window settings workspace without a dialog card", () => {
    const html = renderToString(
      React.createElement(DesktopSettingsDialog, {
        open: true,
        runtimeClient: {} as RuntimeClient,
        theme: "light",
        themePreference: "system",
        fontScale: 100,
        themeSaving: false,
        themeMessage: null,
        onThemePreferenceChange: vi.fn(),
        onFontScaleChange: vi.fn(),
        onOpenChange: vi.fn(),
        onProfilesChanged: vi.fn(),
      }),
    );

    expect(html).toContain("aw-settings-shell--screen");
    expect(html).toContain("aw-settings-shell--models");
    expect(html).toContain("aw-settings-shell--standard");
    expect(html).toContain("配置模型接入、连接状态与推理参数。");
    expect(html).not.toContain("aw-settings-shell--full-bleed");
    expect(html).not.toContain("aw-model-orbit");
    expect(html).toContain("aw-settings-sidebar");
    expect(html).toContain("返回应用");
    expect(html).toContain('role="dialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain("Coding Plan");
    expect(html).toContain("API");
    expect(html).toContain("Agent Plan");
    expect(html).toContain("自定义");
    expect(html).toContain("全局规则");
    expect(html).not.toContain("图片视频工具");
    expect(html).toContain("aw-model-settings__rail");
    expect(html).toContain("配置 Token");
    expect(html).toContain("LLM 设置");
    expect(html).not.toContain("默认推理强度");
    expect(html).not.toContain("aw-dialog__body");
    expect(html).not.toContain("aw-dialog__foot");
  });
});

describe("AppearanceSettings", () => {
  it("renders system, light, and dark theme choices with the selected state", () => {
    const html = renderToString(
      React.createElement(AppearanceSettings, {
        themePreference: "system",
        fontScale: 100,
        themeSaving: false,
        onThemePreferenceChange: vi.fn(),
        onFontScaleChange: vi.fn(),
      }),
    );

    expect(html).toContain("跟随系统");
    expect(html).toContain("显示大小");
    expect(html).toContain("100%");
    expect(html).toContain("浅色");
    expect(html).toContain("黑色");
    expect(html).toContain('role="radiogroup"');
    expect(html.match(/aria-checked="false"/g)).toHaveLength(2);
    expect(html).toContain('aria-checked="true"');
    expect(html).toContain("theme-choice-active");
  });
});

describe("LLM settings cards", () => {
  it("keeps reasoning effort out of settings and inside the Composer flow", () => {
    const html = renderToString(
      React.createElement(AwakenConfiguredModels, {
        models: [
          {
            id: "deepseek",
            displayName: "DeepSeek",
            brand: "deepseek",
            modelId: "deepseek-v4-pro",
            enabled: true,
            contextWindow: 1_000_000,
            contextWindowInput: "1000000",
            contextWindowMin: 8_000,
            contextWindowMax: 1_000_000,
            contextFill: 100,
            contextAnchor: 60,
            contextAnchorValue: 600_000,
            contextAnchorLabel: "600K",
            contextEndLabel: "1M",
            compressionValue: 885_600,
            maxTokensInput: "384000",
          },
        ],
      }),
    );

    expect(html).toContain("configured-model-card");
    expect(html).toContain("LLM 参数");
    expect(html).toContain("DeepSeek Logo");
    expect(html).toContain("DeepSeek：deepseek-v4-pro");
    expect(html).toContain("上下文长度");
    expect(html).toContain("configured-parameter--compression");
    expect(html).not.toContain("Function Calling");
    expect(html).not.toContain("configured-model-access-type");
    expect(html).not.toContain("默认推理强度");
    expect(html).not.toContain("aria-pressed");
  });
});

describe("provider connection cards", () => {
  it("uses the official provider logo and moves the compact form into a centered modal", () => {
    const cardHtml = renderToString(
      React.createElement(AwakenSettingsProviderCard, {
        name: "DeepSeek",
        brand: "deepseek",
        selected: true,
      }),
    );
    const html = renderToString(
      React.createElement(
        React.Fragment,
        null,
        React.createElement(AwakenSettingsProviderCard, {
          name: "DeepSeek",
          brand: "deepseek",
          selected: true,
        }),
        React.createElement(AwakenSettingsProviderModal, {
          open: true,
          name: "DeepSeek",
          brand: "deepseek",
          status: "connected",
          onClose: vi.fn(),
          children: React.createElement(AwakenModelProviderForm, {
            providerId: "deepseek",
            presentation: "modal",
            modelMode: "select",
            models: ["deepseek-v4-pro"],
            modelId: "deepseek-v4-pro",
            baseURL: "https://api.deepseek.com",
            apiKey: "",
            apiKeyPlaceholder: "输入 DEEPSEEK_API_KEY",
            apiDocsURL: "https://platform.deepseek.com/api_keys",
            onModelChange: vi.fn(),
            onBaseURLChange: vi.fn(),
            onApiKeyChange: vi.fn(),
          }),
        }),
      ),
    );

    expect(cardHtml).toContain("DeepSeek Logo");
    expect(cardHtml).toContain("配置");
    expect(cardHtml).not.toContain("已连接");
    expect(cardHtml).not.toContain("个模型");
    expect(cardHtml).not.toContain("正在配置");
    expect(html).toContain("DeepSeek Logo");
    expect(html).toContain("model-provider-modal");
    expect(html).toContain('tabindex="-1"');
    expect(html).toContain("model-provider-form-footer");
    expect(html).not.toContain("高级设置");
    expect(html).not.toContain("Function Calling");
    expect(html).toContain("保存连接");
    expect(html).toContain("已连接");
  });
});

describe("model credential recovery draft", () => {
  it("updates the existing global profile instead of creating a duplicate", () => {
    const provider = {
      id: "deepseek",
      displayName: "DeepSeek",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      supportedProtocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      officialProtocols: ["openai_chat"],
      protocolBaseURLs: {},
      protocolAuthSchemes: {},
      catalogStrategy: "openai",
      catalogSource: "official_manifest",
      authScheme: "bearer",
      keyEnvName: "DEEPSEEK_API_KEY",
      defaultModel: "deepseek-v4-pro",
      models: ["deepseek-v4-pro"],
      docsURL: "https://example.com/docs",
      apiDocsURL: "https://example.com/keys",
      codingPreset: false,
      endpointModel: false,
      allowCustomModel: false,
      modelCapabilities: {},
    } satisfies ModelProvider;
    const profile = {
      id: "profile-1",
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      hasKey: true,
      enabled: true,
      isDefault: true,
      profileStatus: "unreadable",
    } satisfies RuntimeModelProfileSummary;
    const draft = draftFromExistingProfile(profile, provider);
    expect(draft.requiresApiKey).toBe(true);
    draft.apiKey = "replacement-key";

    expect(toSaveRequest(draft, provider)).toMatchObject({
      id: "profile-1",
      templateProfileId: undefined,
      apiKey: "replacement-key",
    });
  });

  it("keeps direct Ark Model IDs separate from optional ep-* selectors", () => {
    const provider = {
      id: "doubao",
      displayName: "Doubao Ark",
      baseURL: "https://ark.cn-beijing.volces.com/api/v3",
      protocol: "openai_chat",
      supportedProtocols: ["openai_chat", "openai_responses"],
      officialProtocols: ["openai_chat", "openai_responses"],
      protocolBaseURLs: {},
      protocolAuthSchemes: {},
      catalogStrategy: "none",
      catalogSource: "official_manifest",
      authScheme: "bearer",
      keyEnvName: "ARK_API_KEY",
      defaultModel: "doubao-seed-2-1-pro-260628",
      models: ["doubao-seed-2-1-pro-260628"],
      docsURL: "",
      apiDocsURL: "",
      accessType: "api",
      codingPreset: false,
      endpointModel: true,
      allowCustomModel: false,
      modelCapabilities: {},
    } satisfies ModelProvider;
    const directProfile = {
      id: "doubao-direct",
      providerId: "doubao",
      displayName: "Doubao Ark",
      modelId: "doubao-seed-2-1-pro-260628",
      baseURL: provider.baseURL,
      protocol: "openai_chat",
      authScheme: "bearer",
      hasKey: true,
      enabled: true,
      isDefault: true,
      profileStatus: "ready",
    } satisfies RuntimeModelProfileSummary;
    const endpointProfile = {
      ...directProfile,
      id: "doubao-endpoint",
      modelId: "ep-custom-endpoint",
      advanced: { endpointId: "ep-custom-endpoint" },
    } satisfies RuntimeModelProfileSummary;

    const directDraft = draftFromExistingProfile(directProfile, provider);
    expect(directDraft.endpointId).toBe("");
    expect(toSaveRequest(directDraft, provider).advanced?.endpointId).toBeUndefined();

    const endpointDraft = draftFromExistingProfile(endpointProfile, provider);
    expect(endpointDraft.endpointId).toBe("ep-custom-endpoint");
    expect(toSaveRequest(endpointDraft, provider)).toMatchObject({
      modelId: "ep-custom-endpoint",
      advanced: { endpointId: "ep-custom-endpoint" },
    });
  });
});

describe("model protocol recommendations", () => {
  it("filters only models with an explicit incompatible contract and keeps unknown models selectable", () => {
    const provider = {
      id: "example",
      displayName: "Example",
      baseURL: "https://example.com/v1",
      protocol: "openai_chat",
      supportedProtocols: ["openai_chat", "openai_responses"],
      officialProtocols: ["openai_chat", "openai_responses"],
      protocolBaseURLs: {},
      protocolAuthSchemes: {},
      catalogStrategy: "openai",
      catalogSource: "provider_api",
      authScheme: "bearer",
      keyEnvName: "EXAMPLE_API_KEY",
      defaultModel: "chat-only",
      models: ["chat-only", "responses-ready", "future-model"],
      docsURL: "",
      apiDocsURL: "",
      codingPreset: false,
      endpointModel: false,
      allowCustomModel: true,
      modelCapabilities: {
        "chat-only": { supportedProtocols: ["openai_chat"] },
        "responses-ready": { supportedProtocols: ["openai_chat", "openai_responses"] },
      },
    } satisfies ModelProvider;

    expect(modelCandidatesForProtocol(provider, "openai_responses")).toEqual([
      "responses-ready",
      "future-model",
    ]);
  });
});
