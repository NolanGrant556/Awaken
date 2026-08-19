import { afterEach, describe, expect, it } from "vitest";
import {
  capabilityForModel,
  normalizeAdvancedWithModelCapability,
  resolveReasoningEffort,
  setCapabilityOverrides,
  validateReasoningEffort,
} from "./model-capabilities.js";
import type { CapabilityResolution } from "./model-capability-sync.js";

function resolution(overrides: Partial<CapabilityResolution["scope"]> = {}): CapabilityResolution {
  return {
    scope: {
      provider_id: "kimi-coding-plan",
      normalized_base_url: "https://api.kimi.com/coding/v1",
      protocol_family: "openai_chat",
      profile_id: "profile-1",
      credential_revision: 1,
      selector_kind: "model",
      endpoint_contract_id: "openai_chat_completions_v1",
      ...overrides,
    },
    selector: "k3",
  };
}

describe("model capabilities", () => {
  afterEach(() => {
    setCapabilityOverrides([]);
  });

  it("knows DeepSeek V4 context and output limits", () => {
    expect(capabilityForModel("deepseek", "deepseek-v4-pro")).toMatchObject({
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: 8192,
      supports_vision: false,
      supports_video: false,
    });
  });

  it("keeps confirmed model protocols separate from provider-wide compatibility", () => {
    expect(capabilityForModel("qwen", "qwen3.8-max")).toMatchObject({
      supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
    });
    expect(capabilityForModel("google", "gemini-3.6-flash")).toMatchObject({
      supported_protocols: ["openai_chat"],
    });
    expect(capabilityForModel("deepseek", "deepseek-v4-pro")?.supported_protocols).toEqual([
      "openai_chat",
      "anthropic_messages",
    ]);
    expect(capabilityForModel("deepseek", "deepseek-v4-flash")?.supported_protocols).toEqual([
      "openai_chat",
      "openai_responses",
      "anthropic_messages",
    ]);
    expect(capabilityForModel("deepseek", "deepseek-v4-flash")?.reasoning_effort).toEqual({
      levels: ["high", "max"],
      default_level: "high",
    });
  });

  it("knows all Kimi Code plan model limits", () => {
    expect(capabilityForModel("kimi-coding-plan", "k3")).toMatchObject({
      context_window: 1024 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
    });
    expect(
      capabilityForModel("kimi-coding-plan", "kimi-for-coding"),
    ).toMatchObject({
      context_window: 256 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
    });
    expect(
      capabilityForModel("kimi-coding-plan", "kimi-for-coding-highspeed"),
    ).toMatchObject({
      context_window: 256 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
    });
  });

  it("merges dynamic Kimi metadata without dropping static output and media capabilities", () => {
    const exact = resolution();
    setCapabilityOverrides([{ scope: exact.scope, models: {
      k3: { context_window: 1024 * 1024 },
    } }]);

    expect(capabilityForModel("kimi-coding-plan", "k3", exact)).toMatchObject({
      context_window: 1024 * 1024,
      max_output_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
    });
  });

  it("keeps provider-native effort labels when an older dynamic cache lacks them", () => {
    const exact = resolution({
      provider_id: "qwen",
      profile_id: "qwen-profile",
      normalized_base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1",
      protocol_family: "openai_responses",
      endpoint_contract_id: "openai_responses_v1",
    });
    exact.selector = "qwen3.8-max";
    setCapabilityOverrides([{ scope: exact.scope, models: {
      "qwen3.8-max": {
        reasoning_effort: {
          levels: ["low", "high", "max"],
          default_level: "max",
        },
      },
    } }]);

    expect(capabilityForModel("qwen", "qwen3.8-max", exact)?.reasoning_effort)
      .toEqual({
        levels: ["low", "high", "max"],
        default_level: "max",
        display_labels: { low: "low", high: "medium", max: "xhigh" },
      });
  });

  it("keeps unknown tool capability and exact negative observations conservative", () => {
    const exact = resolution({ provider_id: "volcengine-agent-plan", profile_id: "agent-profile" });
    exact.selector = "custom-model";
    setCapabilityOverrides([{ scope: exact.scope, models: {
      "custom-model": { supports_tools: "unsupported" },
    } }]);

    expect(capabilityForModel("minimax", "MiniMax-M3")?.supports_tools).toBe("ready");
    expect(capabilityForModel("volcengine-agent-plan", "custom-model")).toBeUndefined();
    expect(
      capabilityForModel("volcengine-agent-plan", "custom-model", exact)?.supports_tools,
    ).toBe("unsupported");
    expect(
      normalizeAdvancedWithModelCapability(
        { supportsTools: "ready" },
        "qwen",
        "custom-model",
      ).supportsTools,
    ).toBeUndefined();
  });

  it("uses the current official MiniMax M3 and GLM 5.2 token limits", () => {
    expect(capabilityForModel("minimax", "MiniMax-M3")).toMatchObject({
      context_window: 1_000_000,
      max_output_tokens: 128_000,
      supports_prompt_cache: true,
    });
    expect(capabilityForModel("glm", "glm-5.2")).toMatchObject({
      context_window: 1_000_000,
      max_output_tokens: 128_000,
    });
  });

  it("knows Volcengine Coding Plan model limits", () => {
    expect(
      capabilityForModel("volcengine", "doubao-seed-2.0-pro"),
    ).toMatchObject({
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
      maxTokens: 32 * 1024,
      supportsVision: true,
      supportsVideo: true,
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

  it("defaults custom visual input off and only accepts an explicit custom override", () => {
    expect(
      normalizeAdvancedWithModelCapability(undefined, "custom", "custom-model"),
    ).toMatchObject({
      supportsVision: false,
      supportsNativePDF: false,
    });

    expect(
      normalizeAdvancedWithModelCapability(
        { supportsVision: true, supportsNativePDF: true },
        "custom",
        "custom-model",
      ),
    ).toMatchObject({
      supportsVision: true,
      supportsNativePDF: false,
    });
  });

  it("keeps built-in capabilities authoritative when legacy switches disagree", () => {
    const advanced = normalizeAdvancedWithModelCapability(
      { supportsVision: false, supportsNativePDF: true },
      "openai",
      "gpt-5.6-sol",
      undefined,
      "openai_chat",
    );

    expect(advanced.supportsVision).toBe(true);
    expect(advanced.supportsNativePDF).toBeUndefined();
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

  it("does not invent token limits for unknown Volcengine model names", () => {
    const advanced = normalizeAdvancedWithModelCapability(
      undefined,
      "volcengine",
      "new-model-from-console",
    );
    expect(advanced.contextWindow).toBeUndefined();
    expect(advanced.maxTokens).toBeUndefined();
    expect(advanced.supportsTools).toBeUndefined();
  });

  it("fills Kimi Code temperature from model capability", () => {
    expect(
      normalizeAdvancedWithModelCapability(
        undefined,
        "kimi-coding-plan",
        "kimi-for-coding",
      ),
    ).toMatchObject({
      temperature: 1,
      contextWindow: 256 * 1024,
      maxTokens: 32 * 1024,
      supportsVision: true,
      supportsVideo: true,
    });
  });

  it("uses the configured default output budget instead of the hard model ceiling", () => {
    expect(
      normalizeAdvancedWithModelCapability(undefined, "kimi-cn", "kimi-k3"),
    ).toMatchObject({
      maxTokens: 32 * 1024,
    });
  });

  it("exposes only confirmed Awaken effort levels", () => {
    expect(capabilityForModel("kimi-cn", "kimi-k3")?.reasoning_effort).toEqual({
      levels: ["low", "high", "max"],
      default_level: "max",
    });
    expect(capabilityForModel("glm", "glm-5.2")?.reasoning_effort).toEqual({
      levels: ["high", "max"],
      default_level: "max",
    });
    expect(capabilityForModel("glm-coding-plan", "glm-5.2")?.reasoning_effort).toEqual({
      levels: ["high", "max"],
      default_level: "max",
    });
    expect(capabilityForModel("qwen-token-plan", "qwen3.8-max-preview")?.reasoning_effort)
      .toEqual({
        levels: ["low", "high", "max"],
        default_level: "max",
        display_labels: { low: "low", high: "medium", max: "xhigh" },
      });
    expect(capabilityForModel("qwen", "qwen3.8-max")?.reasoning_effort)
      .toEqual({
        levels: ["low", "high", "max"],
        default_level: "max",
        display_labels: { low: "low", high: "medium", max: "xhigh" },
      });
    expect(capabilityForModel("minimax", "MiniMax-M3")?.reasoning_effort).toBeUndefined();
    expect(capabilityForModel("qwen", "qwen3.7-max")?.reasoning_effort).toBeUndefined();
    expect(capabilityForModel("volcengine-agent-plan", "ark-code-latest")?.reasoning_effort)
      .toBeUndefined();
  });

  it("falls back from stale effort to profile and capability defaults", () => {
    expect(resolveReasoningEffort("glm", "glm-5.2", "low", "max")).toBe("max");
    expect(resolveReasoningEffort("glm", "glm-5.2", "low", "low")).toBe("max");
    expect(resolveReasoningEffort("minimax", "MiniMax-M3", "max", "max")).toBeUndefined();
  });

  it("rejects unsupported profile defaults", () => {
    expect(() => validateReasoningEffort("glm", "glm-5.2", "low")).toThrow(
      "当前模型不支持推理强度 low",
    );
    expect(() => validateReasoningEffort("glm", "glm-5.2", "high")).not.toThrow();
    expect(() => validateReasoningEffort("qwen", "qwen3.7-max", "max")).toThrow();
  });

  it("does not infer Doubao reasoning capability from an opaque endpoint selector", () => {
    expect(capabilityForModel("doubao", "ep-arbitrary")?.reasoning_effort).toBeUndefined();
    expect(capabilityForModel("doubao", "doubao-seed-2-1-pro-260628")?.reasoning_effort).toEqual({
      levels: ["low", "high", "max"],
      default_level: "high",
    });
  });

  it("does not reuse official dynamic metadata for a custom base URL", () => {
    const official = resolution();
    const custom = resolution({ normalized_base_url: "https://proxy.example.test/v1" });
    setCapabilityOverrides([{ scope: official.scope, models: {
      k3: { context_window: 64_000 },
    } }]);
    expect(capabilityForModel("kimi-coding-plan", "k3", official)?.context_window).toBe(64_000);
    expect(capabilityForModel("kimi-coding-plan", "k3", custom)?.context_window).toBe(1024 * 1024);
  });
});
