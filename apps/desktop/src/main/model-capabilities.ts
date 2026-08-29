import type {
  ModelCapabilityWire,
  ModelProfileAdvanced,
  ModelProviderWire,
  ReasoningEffortCapabilityWire,
  ReasoningEffortLevel,
} from "../shared/runtime.js";
import {
  capabilityScopeKey,
  type CapabilityResolution,
  type ScopedCapabilityOverrides,
} from "./model-capability-sync.js";

let dynamicCapabilityOverrides: ScopedCapabilityOverrides[] = [];

const DEFAULT_MAX_TOKENS = 8_192;

export function setCapabilityOverrides(
  overrides: ScopedCapabilityOverrides[],
): void {
  dynamicCapabilityOverrides = overrides;
}

const VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW = 256_000;
const VOLCENGINE_CODING_PLAN_HIGH_OUTPUT = 128_000;
const VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT = 32_000;

const MODEL_CAPABILITIES: Record<string, Record<string, ModelCapabilityWire>> = {
  openai: {
    "gpt-5.1": {
      supported_protocols: ["openai_chat", "openai_responses"],
      context_window: 400_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "gpt-5-mini": {
      supported_protocols: ["openai_chat", "openai_responses"],
      context_window: 400_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high"], default_level: "high" },
    },
    "gpt-5.6-sol": {
      supported_protocols: ["openai_chat", "openai_responses"],
      context_window: 1_050_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "gpt-5.6-terra": {
      supported_protocols: ["openai_chat", "openai_responses"],
      context_window: 1_050_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "gpt-5.6-luna": {
      supported_protocols: ["openai_chat", "openai_responses"],
      context_window: 1_050_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
  },
  anthropic: {
    "claude-sonnet-4-6": {
      supported_protocols: ["anthropic_messages"],
      max_input_tokens: 1_000_000,
      max_output_tokens: 64_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_native_pdf: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "claude-opus-4-6": {
      supported_protocols: ["anthropic_messages"],
      max_input_tokens: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_native_pdf: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "claude-fable-5": {
      supported_protocols: ["anthropic_messages"],
      max_input_tokens: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_native_pdf: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
    },
    "claude-opus-5": {
      supported_protocols: ["anthropic_messages"],
      max_input_tokens: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_native_pdf: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "claude-sonnet-5": {
      supported_protocols: ["anthropic_messages"],
      max_input_tokens: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_native_pdf: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "claude-haiku-4-5": {
      supported_protocols: ["anthropic_messages"],
      max_input_tokens: 200_000,
      max_output_tokens: 64_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_native_pdf: true,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
    },
  },
  google: {
    "gemini-2.5-pro": {
      supported_protocols: ["openai_chat"],
      max_input_tokens: 1_048_576,
      max_output_tokens: 65_536,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_native_pdf: false,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: false,
      reasoning_effort: { levels: ["low", "high"], default_level: "high" },
    },
    "gemini-2.5-flash": {
      supported_protocols: ["openai_chat"],
      max_input_tokens: 1_048_576,
      max_output_tokens: 65_536,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_native_pdf: false,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: false,
      reasoning_effort: { levels: ["low", "high"], default_level: "high" },
    },
    "gemini-3.6-flash": {
      supported_protocols: ["openai_chat"],
      max_input_tokens: 1_048_576,
      max_output_tokens: 65_536,
      default_max_tokens: 32_768,
      supports_vision: true,
      // Awaken calls Gemini through Google's OpenAI-compatible Chat endpoint.
      // That contract documents image input but not PDF/video input, even
      // though the native Gemini API supports both.
      supports_native_pdf: false,
      supports_video: false,
      supports_tools: "ready",
      // Compatibility exposes Google's cached_content extension, not
      // OpenAI's prompt_cache_key. Awaken does not create cachedContent
      // resources yet, so this protocol contract stays disabled.
      supports_prompt_cache: false,
      reasoning_effort: { levels: ["low", "high"], default_level: "high" },
    },
    "gemini-3.5-flash-lite": {
      supported_protocols: ["openai_chat"],
      max_input_tokens: 1_048_576,
      max_output_tokens: 65_536,
      default_max_tokens: 32_768,
      supports_vision: true,
      supports_native_pdf: false,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: false,
      reasoning_effort: { levels: ["low", "high"], default_level: "high" },
    },
  },
  deepseek: {
    "deepseek-v4-pro": {
      supported_protocols: ["openai_chat", "anthropic_messages"],
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: DEFAULT_MAX_TOKENS,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      reasoning_effort: { levels: ["high", "max"], default_level: "high" },
    },
    "deepseek-v4-flash": {
      supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: DEFAULT_MAX_TOKENS,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      reasoning_effort: { levels: ["high", "max"], default_level: "high" },
    },
  },
  "kimi-cn": {
    "kimi-k3": {
      context_window: 1024 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "max" },
    },
    "kimi-k2.7-code": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
      supports_vision: true,
      supports_video: true,
    },
    "kimi-k2.7-code-highspeed": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
      supports_vision: true,
      supports_video: true,
    },
    "kimi-k2.6": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
      supports_vision: true,
      supports_video: true,
    },
    "kimi-k2.5": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
      supports_vision: true,
      supports_video: true,
    },
    "moonshot-v1-128k": {
      context_window: 128 * 1024,
      max_output_tokens: 128 * 1024,
      default_max_tokens: DEFAULT_MAX_TOKENS,
      supports_vision: true,
      supports_video: true,
    },
    "moonshot-v1-32k": {
      context_window: 32 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: DEFAULT_MAX_TOKENS,
      supports_vision: true,
      supports_video: true,
    },
    "moonshot-v1-8k": {
      context_window: 8 * 1024,
      max_output_tokens: 8 * 1024,
      default_max_tokens: 4096,
      supports_vision: true,
      supports_video: true,
    },
  },
  "kimi-coding-plan": {
    k3: {
      context_window: 1024 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "k3-256k": {
      context_window: 256 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    "kimi-for-coding": {
      context_window: 256 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
    },
    "kimi-for-coding-highspeed": {
      context_window: 256 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: 32 * 1024,
      default_temperature: 1,
      supports_vision: true,
      supports_video: true,
    },
  },
  volcengine: {
    "ark-code-latest": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
    },
    "doubao-seed-code": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: true,
      supports_video: false,
    },
    "doubao-seed-2.0-code": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      supports_vision: true,
      supports_video: false,
    },
    "doubao-seed-2.0-pro": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      supports_vision: true,
      supports_video: false,
    },
    "doubao-seed-2.0-lite": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      supports_vision: true,
      supports_video: false,
    },
    "minimax-m2.5": {
      context_window: 200_000,
      max_output_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      supports_vision: true,
      supports_video: false,
    },
    "minimax-m2.7": {
      context_window: 256_000,
      max_output_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      supports_vision: true,
      supports_video: false,
    },
    "minimax-m3": {
      context_window: 256_000,
      max_output_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      supports_vision: true,
      supports_video: false,
    },
    "glm-4.7": {
      context_window: 200_000,
      max_output_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_HIGH_OUTPUT,
      supports_vision: false,
      supports_video: false,
    },
    "glm-5.2": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: false,
      supports_video: false,
    },
    "glm-latest": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: false,
      supports_video: false,
    },
    "deepseek-v3.2": {
      context_window: 128_000,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: false,
      supports_video: false,
    },
    "deepseek-v4-flash": {
      context_window: 128_000,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: false,
      supports_video: false,
    },
    "deepseek-v4-pro": {
      context_window: 128_000,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: false,
      supports_video: false,
    },
    "kimi-k2.5": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: true,
      supports_video: true,
    },
    "kimi-k2.6": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: true,
      supports_video: true,
    },
    "kimi-k2.7-code": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: true,
      supports_video: true,
    },
  },
  minimax: {
    "MiniMax-M3": {
      supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      context_window: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      supports_prompt_cache: true,
    },
  },
  "minimax-token-plan": {
    "MiniMax-M3": {
      supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      context_window: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      supports_prompt_cache: true,
    },
    "MiniMax-M2.7": {
      context_window: 200_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
    },
    "MiniMax-M2.5": {
      context_window: 200_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      supports_prompt_cache: true,
    },
  },
  glm: {
    "glm-5.2": {
      context_window: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      reasoning_effort: { levels: ["high", "max"], default_level: "max" },
    },
  },
  "glm-coding-plan": {
    "glm-5.2": {
      context_window: 1_000_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      reasoning_effort: { levels: ["high", "max"], default_level: "max" },
    },
    "glm-5-turbo": {
      context_window: 200_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
    },
    "glm-4.7": {
      context_window: 200_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
    },
  },
  qwen: {
    "qwen3.7-plus": {
      supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      context_window: 1_000_000,
      max_input_tokens: 983_616,
      max_output_tokens: 131_072,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
    },
    "qwen3.8-max": {
      supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      max_input_tokens: 983_616,
      context_window: 1024 * 1024,
      max_output_tokens: 64_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
      reasoning_effort: {
        levels: ["low", "high", "max"],
        default_level: "max",
        display_labels: { low: "low", high: "medium", max: "xhigh" },
      },
    },
    "qwen3.7-max": {
      context_window: 256_000,
      max_output_tokens: 64_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
    },
  },
  "qwen-token-plan": {
    "qwen3.8-max-preview": {
      supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      context_window: 1024 * 1024,
      max_output_tokens: 64_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      reasoning_effort: {
        levels: ["low", "high", "max"],
        default_level: "max",
        display_labels: { low: "low", high: "medium", max: "xhigh" },
      },
    },
  },
  doubao: {
    "doubao-seed-2-1-pro-260628": {
      supported_protocols: ["openai_chat", "openai_responses"],
      context_window: 256_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
    // Read compatibility for profiles created before Ark exposed exact,
    // versioned Model IDs in the provider manifest.
    "doubao-seed-2.1-pro": {
      supported_protocols: ["openai_chat", "openai_responses"],
      context_window: 256_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "high" },
    },
  },
  "volcengine-agent-plan": {
    "ark-code-latest": {
      context_window: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
      max_output_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      default_max_tokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
      supports_vision: true,
      supports_video: false,
      supports_tools: "ready",
    },
  },
};

export function reasoningEffortCapability(
  providerId: string,
  modelId: string,
  resolution?: CapabilityResolution,
): ReasoningEffortCapabilityWire | undefined {
  return capabilityForModel(providerId, modelId, resolution)?.reasoning_effort;
}

export function validateReasoningEffort(
  providerId: string,
  modelId: string,
  effort: ReasoningEffortLevel | undefined,
  resolution?: CapabilityResolution,
): void {
  if (!effort) return;
  const capability = reasoningEffortCapability(providerId, modelId, resolution);
  if (!capability?.levels.includes(effort)) {
    throw new Error(`当前模型不支持推理强度 ${effort}`);
  }
}

export function resolveReasoningEffort(
  providerId: string,
  modelId: string,
  requested: ReasoningEffortLevel | undefined,
  profileDefault: ReasoningEffortLevel | undefined,
  resolution?: CapabilityResolution,
): ReasoningEffortLevel | undefined {
  const capability = reasoningEffortCapability(providerId, modelId, resolution);
  if (!capability) return undefined;
  if (requested && capability.levels.includes(requested)) return requested;
  if (profileDefault && capability.levels.includes(profileDefault)) return profileDefault;
  return capability.default_level;
}

function dynamicCapabilityForResolution(
  resolution: CapabilityResolution | undefined,
): ModelCapabilityWire | undefined {
  if (!resolution) return undefined;
  const key = capabilityScopeKey(resolution.scope);
  return dynamicCapabilityOverrides.find(
    (entry) => capabilityScopeKey(entry.scope) === key,
  )?.models[resolution.selector];
}

export function capabilitiesForProvider(
  providerId: string,
  resolution?: CapabilityResolution,
): Record<string, ModelCapabilityWire> {
  const builtin = MODEL_CAPABILITIES[providerId] ?? {};
  const dynamic = resolution
    ? { [resolution.selector]: dynamicCapabilityForResolution(resolution) ?? {} }
    : {};
  return Object.fromEntries(
    Array.from(new Set([...Object.keys(builtin), ...Object.keys(dynamic)])).map(
      (modelId) => [
        modelId,
        mergeCapabilitiesWithSources(builtin[modelId], dynamic[modelId]),
      ],
    ),
  );
}

export function capabilityForModel(
  providerId: string,
  modelId: string,
  resolution?: CapabilityResolution,
): ModelCapabilityWire | undefined {
  const builtin = MODEL_CAPABILITIES[providerId]?.[modelId];
  const dynamic = dynamicCapabilityForResolution(resolution);
  if (!builtin && !dynamic) return undefined;
  return mergeCapabilitiesWithSources(builtin, dynamic);
}

function capabilityFields(capability?: ModelCapabilityWire): Array<keyof ModelCapabilityWire> {
  if (!capability) return [];
  return Object.keys(capability).filter((field) => field !== "field_sources") as Array<keyof ModelCapabilityWire>;
}

function publicCapabilityField(field: keyof ModelCapabilityWire): string | undefined {
  return ({
    supported_protocols: "supportedProtocols",
    max_input_tokens: "maxInputTokens",
    context_window: "contextWindow",
    max_output_tokens: "maxOutputTokens",
    supports_vision: "supportsVision",
    supports_native_pdf: "supportsNativePDF",
    supports_video: "supportsVideo",
    supports_tools: "supportsTools",
    supports_prompt_cache: "supportsPromptCache",
    reasoning_effort: "reasoningEffort",
  } as Partial<Record<keyof ModelCapabilityWire, string>>)[field];
}

function mergeCapabilitiesWithSources(
  builtin?: ModelCapabilityWire,
  dynamic?: ModelCapabilityWire,
): ModelCapabilityWire {
  const fieldSources: NonNullable<ModelCapabilityWire["field_sources"]> = {
    ...(builtin?.field_sources ?? {}),
    ...(dynamic?.field_sources ?? {}),
  };
  for (const field of capabilityFields(builtin)) {
    const publicField = publicCapabilityField(field);
    if (publicField && !fieldSources[publicField as keyof typeof fieldSources]) {
      fieldSources[publicField as keyof typeof fieldSources] = { source: "official_manifest" };
    }
  }
  const reasoningEffort = dynamic?.reasoning_effort
    ? {
        ...builtin?.reasoning_effort,
        ...dynamic.reasoning_effort,
        display_labels:
          dynamic.reasoning_effort.display_labels ??
          builtin?.reasoning_effort?.display_labels,
      }
    : builtin?.reasoning_effort;
  return {
    ...builtin,
    ...dynamic,
    ...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
    field_sources: fieldSources,
  };
}

export function normalizeAdvancedWithModelCapability(
  advanced: ModelProfileAdvanced | undefined,
  providerId: string,
  modelId: string,
  resolution?: CapabilityResolution,
  protocol?: string,
): ModelProfileAdvanced {
  const capability = capabilityForModel(providerId, modelId, resolution);
  const customVision =
    providerId === "custom" ? advanced?.supportsVision ?? false : undefined;
  const inferredMaxTokens =
    capability?.default_max_tokens ?? capability?.max_output_tokens;
  const requestedMaxTokens = advanced?.maxTokens ?? inferredMaxTokens;
  const requestedContextWindow =
    advanced?.contextWindow ?? capability?.context_window;
  return {
    ...advanced,
    // Built-in capability switches are runtime facts, not editable preferences.
    // A custom profile is the narrow exception: the user explicitly declares
    // whether its exact transport contract accepts visual input.
    temperature: capability?.default_temperature,
    supportsVision:
      providerId === "custom" ? customVision : capability?.supports_vision,
    maxInputTokens: capability?.max_input_tokens,
    supportsNativePDF:
      providerId === "custom"
        ? false
        : providerId === "openai" && protocol === "openai_responses"
          ? true
          : capability?.supports_native_pdf,
    supportsVideo: capability?.supports_video,
    supportsTools: capability?.supports_tools,
    supportsPromptCache: capability?.supports_prompt_cache,
    defaultReasoningEffort: resolveReasoningEffort(
      providerId,
      modelId,
      undefined,
      undefined,
      resolution,
    ),
    maxTokens:
      requestedMaxTokens !== undefined && capability?.max_output_tokens !== undefined
        ? Math.min(requestedMaxTokens, capability.max_output_tokens)
        : requestedMaxTokens,
    contextWindow:
      requestedContextWindow !== undefined && capability?.context_window !== undefined
        ? Math.min(requestedContextWindow, capability.context_window)
        : requestedContextWindow,
  };
}

export function effectiveCapabilityForProfile(
  advanced: ModelProfileAdvanced,
  providerId: string,
  modelId: string,
  resolution?: CapabilityResolution,
  protocol?: string,
): ModelCapabilityWire | undefined {
  const base = capabilityForModel(providerId, modelId, resolution);
  const effective: ModelCapabilityWire = {
    ...(base ?? {}),
    field_sources: { ...(base?.field_sources ?? {}) },
  };
  if (providerId === "openai" && protocol === "openai_responses") {
    effective.supports_native_pdf = true;
    effective.field_sources!.supportsNativePDF = { source: "official_manifest" };
  }
  const overrides = [
    ["max_input_tokens", "maxInputTokens", advanced.maxInputTokens],
    ["context_window", "contextWindow", advanced.contextWindow],
    ["supports_vision", "supportsVision", advanced.supportsVision],
    ["supports_native_pdf", "supportsNativePDF", advanced.supportsNativePDF],
    ["supports_video", "supportsVideo", advanced.supportsVideo],
    ["supports_tools", "supportsTools", advanced.supportsTools],
    ["supports_prompt_cache", "supportsPromptCache", advanced.supportsPromptCache],
  ] as const;
  for (const [wireField, publicField, value] of overrides) {
    if (value === undefined) continue;
    if (effective[wireField] !== value) {
      effective.field_sources![publicField] = { source: "user_override" };
    }
    (effective as Record<string, unknown>)[wireField] = value;
  }
  return Object.keys(effective).some((field) => field !== "field_sources")
    ? effective
    : undefined;
}

export function withModelCapabilities(provider: ModelProviderWire): ModelProviderWire {
  const modelCapabilities = capabilitiesForProvider(provider.id);
  return Object.keys(modelCapabilities).length > 0
    ? { ...provider, model_capabilities: modelCapabilities }
    : provider;
}
