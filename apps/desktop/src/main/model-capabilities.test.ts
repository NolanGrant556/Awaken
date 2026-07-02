import { describe, expect, it } from "vitest";
import {
  capabilityForModel,
  normalizeAdvancedWithModelCapability,
} from "./model-capabilities.js";

describe("model capabilities", () => {
  it("knows DeepSeek V4 context and output limits", () => {
    expect(capabilityForModel("deepseek", "deepseek-v4-pro")).toMatchObject({
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: 8192,
    });
  });

  it("fills profile defaults from the selected model without replacing user overrides", () => {
    expect(
      normalizeAdvancedWithModelCapability(undefined, "kimi-cn", "kimi-k2.6"),
    ).toMatchObject({
      contextWindow: 256 * 1024,
      maxTokens: 256 * 1024,
    });

    expect(
      normalizeAdvancedWithModelCapability(
        { contextWindow: 400_000, maxTokens: 12_000 },
        "kimi-cn",
        "kimi-k2.6",
      ),
    ).toMatchObject({
      contextWindow: 400_000,
      maxTokens: 12_000,
    });
  });

  it("upgrades previously generated default max tokens to the known model output limit", () => {
    expect(
      normalizeAdvancedWithModelCapability(
        { contextWindow: 1_000_000, maxTokens: 8192 },
        "deepseek",
        "deepseek-v4-pro",
      ),
    ).toMatchObject({
      contextWindow: 1_000_000,
      maxTokens: 384_000,
    });
  });
});
