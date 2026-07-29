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

export function setCapabilityOverrides(
  overrides: ScopedCapabilityOverrides[],
): void {
  dynamicCapabilityOverrides = overrides;
}

const DEFAULT_CONTEXT_WINDOW = 1_000_000;
const DEFAULT_MAX_TOKENS = 8192;
const VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW = 256_000;
const VOLCENGINE_CODING_PLAN_HIGH_OUTPUT = 128_000;
const VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT = 32_000;

const MODEL_CAPABILITIES: Record<string, Record<string, ModelCapabilityWire>> = {
  deepseek: {
    "deepseek-v4-pro": {
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: DEFAULT_MAX_TOKENS,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      reasoning_effort: { levels: ["high", "max"], default_level: "high" },
    },
    "deepseek-v4-flash": {
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: DEFAULT_MAX_TOKENS,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
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
      context_window: 256_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
    },
  },
  glm: {
    "glm-5.2": {
      context_window: 256_000,
      max_output_tokens: 128_000,
      default_max_tokens: 32_000,
      supports_vision: false,
      supports_video: false,
      supports_tools: "ready",
      reasoning_effort: { levels: ["high", "max"], default_level: "high" },
    },
  },
  qwen: {
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
      context_window: 1024 * 1024,
      max_output_tokens: 64_000,
      default_max_tokens: 32_000,
      supports_vision: true,
      supports_video: true,
      supports_tools: "ready",
      reasoning_effort: { levels: ["low", "high", "max"], default_level: "max" },
    },
  },
  doubao: {
    "doubao-seed-2.1-pro": {
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
        {
          ...builtin[modelId],
          ...dynamic[modelId],
          supports_tools: "ready",
        },
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
  return { ...builtin, ...dynamic, supports_tools: "ready" };
}

export function normalizeAdvancedWithModelCapability(
  advanced: ModelProfileAdvanced | undefined,
  providerId: string,
  modelId: string,
  resolution?: CapabilityResolution,
): ModelProfileAdvanced {
  const capability = capabilityForModel(providerId, modelId, resolution);
  const providerDefault =
    providerId === "volcengine"
      ? {
          contextWindow: VOLCENGINE_CODING_PLAN_CONTEXT_WINDOW,
          maxTokens: VOLCENGINE_CODING_PLAN_STANDARD_OUTPUT,
        }
      : {
          contextWindow: DEFAULT_CONTEXT_WINDOW,
          maxTokens: DEFAULT_MAX_TOKENS,
        };
  const inferredMaxTokens =
    capability?.default_max_tokens ?? capability?.max_output_tokens ?? providerDefault.maxTokens;
  const requestedMaxTokens = advanced?.maxTokens ?? inferredMaxTokens;
  const requestedContextWindow =
    advanced?.contextWindow ?? capability?.context_window ?? providerDefault.contextWindow;
  return {
    ...advanced,
    temperature: advanced?.temperature ?? capability?.default_temperature,
    supportsVision: advanced?.supportsVision ?? capability?.supports_vision,
    supportsNativePDF:
      advanced?.supportsNativePDF ?? capability?.supports_native_pdf ?? false,
    supportsVideo: advanced?.supportsVideo ?? capability?.supports_video,
    supportsTools: "ready",
    defaultReasoningEffort: resolveReasoningEffort(
      providerId,
      modelId,
      undefined,
      advanced?.defaultReasoningEffort,
      resolution,
    ),
    maxTokens:
      capability?.max_output_tokens !== undefined
        ? Math.min(requestedMaxTokens, capability.max_output_tokens)
        : requestedMaxTokens,
    contextWindow:
      capability?.context_window !== undefined
        ? Math.min(requestedContextWindow, capability.context_window)
        : requestedContextWindow,
  };
}

export function withModelCapabilities(provider: ModelProviderWire): ModelProviderWire {
  const modelCapabilities = capabilitiesForProvider(provider.id);
  return Object.keys(modelCapabilities).length > 0
    ? { ...provider, model_capabilities: modelCapabilities }
    : provider;
}
