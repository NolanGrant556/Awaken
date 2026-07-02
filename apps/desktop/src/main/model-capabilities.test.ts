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

  it("knows Kimi Code plan model limits", () => {
    expect(capabilityForModel("kimi-coding-plan", "kimi-for-coding")).toMatchObject({
      context_window: 256 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
    });
  });

  it("knows Volcengine Coding Plan model limits", () => {
    expect(capabilityForModel("volcengine", "doubao-seed-2.0-pro")).toMatchObject({
      context_window: 256_000,
      max_output_tokens: 128_000,
      default_max_tokens: 128_000,
    });
    expect(capabilityForModel("volcengine", "deepseek-v4-pro")).toMatchObject({
      context_window: 128_000,
      max_output_tokens: 32_000,
      default_max_tokens: 32_000,
    });
    expect(capabilityForModel("volcengine", "glm-5.2")).toMatchObject({
      context_window: 256_000,
      max_output_tokens: 32_000,
      default_max_tokens: 32_000,
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
        { contextWindow: 200_000, maxTokens: 12_000 },
        "kimi-cn",
        "kimi-k2.6",
      ),
    ).toMatchObject({
      contextWindow: 200_000,
      maxTokens: 12_000,
    });
  });

  it("caps profile values to selected model limits", () => {
    expect(
      normalizeAdvancedWithModelCapability(
        { contextWindow: 1_000_000, maxTokens: 384_000 },
        "volcengine",
        "doubao-seed-2.0-pro",
      ),
    ).toMatchObject({
      contextWindow: 256_000,
      maxTokens: 128_000,
    });
  });

  it("uses conservative Volcengine defaults for custom Coding Plan model names", () => {
    expect(
      normalizeAdvancedWithModelCapability(undefined, "volcengine", "new-model-from-console"),
    ).toMatchObject({
      contextWindow: 256_000,
      maxTokens: 32_000,
    });
  });

  it("fills Kimi Code temperature from model capability", () => {
    expect(
      normalizeAdvancedWithModelCapability(undefined, "kimi-coding-plan", "kimi-for-coding"),
    ).toMatchObject({
      temperature: 1,
      contextWindow: 256 * 1024,
      maxTokens: 32 * 1024,
    });
  });
});
