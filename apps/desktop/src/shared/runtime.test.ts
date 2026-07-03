import { describe, expect, it } from "vitest";
import {
  calculateAutoCompactThreshold,
  effectiveContextWindow,
  normalizeModelProvider,
} from "./runtime.js";

describe("runtime shared model provider normalization", () => {
  it("normalizes model capability metadata", () => {
    const provider = normalizeModelProvider({
      id: "deepseek",
      display_name: "DeepSeek",
      base_url: "https://api.deepseek.com",
      protocol: "openai_chat",
      auth_scheme: "bearer",
      key_env_name: "DEEPSEEK_API_KEY",
      default_model: "deepseek-v4-pro",
      models: ["deepseek-v4-pro"],
      docs_url: "https://api-docs.deepseek.com/",
      api_docs_url: "https://platform.deepseek.com/api_keys",
      allow_custom_model: true,
      model_capabilities: {
        "deepseek-v4-pro": {
          context_window: 1_000_000,
          max_output_tokens: 384_000,
          default_max_tokens: 8192,
        },
      },
    });

    expect(provider.modelCapabilities["deepseek-v4-pro"]).toEqual({
      contextWindow: 1_000_000,
      maxOutputTokens: 384_000,
      defaultMaxTokens: 8192,
      defaultTemperature: undefined,
    });
    expect(provider.allowCustomModel).toBe(true);
  });

  it("calculates the auto compact threshold from effective context window", () => {
    expect(effectiveContextWindow(1_000_000)).toBe(984_000);
    expect(calculateAutoCompactThreshold(1_000_000)).toBe(885_600);
    expect(calculateAutoCompactThreshold(8_000)).toBe(0);
    expect(calculateAutoCompactThreshold(12_000, 20_000)).toBe(0);
  });
});
