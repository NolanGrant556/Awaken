import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type {
  ModelProvider,
  RuntimeModelProfileSummary,
} from "../../shared/runtime";
import {
  AppearanceSettings,
  draftFromExistingProfile,
  toSaveRequest,
} from "./DesktopSettingsDialog";

describe("AppearanceSettings", () => {
  it("renders system, light, and dark theme choices with the selected state", () => {
    const html = renderToString(
      React.createElement(AppearanceSettings, {
        themePreference: "system",
        themeSaving: false,
        onThemePreferenceChange: vi.fn(),
      }),
    );

    expect(html).toContain("跟随系统");
    expect(html).toContain("浅色");
    expect(html).toContain("黑色");
    expect(html).toContain('role="radiogroup"');
    expect(html.match(/aria-checked="false"/g)).toHaveLength(2);
    expect(html).toContain('aria-checked="true"');
    expect(html).toContain("theme-choice-active");
  });
});

describe("model credential recovery draft", () => {
  it("updates the existing global profile instead of creating a duplicate", () => {
    const provider = {
      id: "deepseek",
      displayName: "DeepSeek",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
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
});
