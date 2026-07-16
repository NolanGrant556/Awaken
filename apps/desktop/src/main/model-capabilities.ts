import type {
  ModelCapabilityWire,
  ModelProfileAdvanced,
  ModelProviderWire,
} from "../shared/runtime.js";
import type { ProviderCapabilityOverrides } from "./model-capability-sync.js";

let dynamicCapabilityOverrides: Record<string, ProviderCapabilityOverrides> = {};

export function setCapabilityOverrides(
  overrides: Record<string, ProviderCapabilityOverrides>,
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
    },
    "deepseek-v4-flash": {
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: DEFAULT_MAX_TOKENS,
      supports_vision: false,
      supports_video: false,
    },
  },
  "kimi-cn": {
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
    "kimi-for-coding": {
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
};

export function capabilitiesForProvider(providerId: string): Record<string, ModelCapabilityWire> {
  return {
    ...(MODEL_CAPABILITIES[providerId] ?? {}),
    ...(dynamicCapabilityOverrides[providerId] ?? {}),
  };
}

export function capabilityForModel(
  providerId: string,
  modelId: string,
): ModelCapabilityWire | undefined {
  return capabilitiesForProvider(providerId)[modelId];
}

export function normalizeAdvancedWithModelCapability(
  advanced: ModelProfileAdvanced | undefined,
  providerId: string,
  modelId: string,
): ModelProfileAdvanced {
  const capability = capabilityForModel(providerId, modelId);
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
    capability?.max_output_tokens ?? capability?.default_max_tokens ?? providerDefault.maxTokens;
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
