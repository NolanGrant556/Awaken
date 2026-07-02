import type {
  ModelCapabilityWire,
  ModelProfileAdvanced,
  ModelProviderWire,
} from "../shared/runtime.js";

const DEFAULT_CONTEXT_WINDOW = 1_000_000;
const DEFAULT_MAX_TOKENS = 8192;

const MODEL_CAPABILITIES: Record<string, Record<string, ModelCapabilityWire>> = {
  deepseek: {
    "deepseek-v4-pro": {
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: DEFAULT_MAX_TOKENS,
    },
    "deepseek-v4-flash": {
      context_window: 1_000_000,
      max_output_tokens: 384_000,
      default_max_tokens: DEFAULT_MAX_TOKENS,
    },
  },
  "kimi-cn": {
    "kimi-k2.7-code": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
    },
    "kimi-k2.7-code-highspeed": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
    },
    "kimi-k2.6": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
    },
    "kimi-k2.5": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
    },
    "moonshot-v1-128k": {
      context_window: 128 * 1024,
      max_output_tokens: 128 * 1024,
      default_max_tokens: DEFAULT_MAX_TOKENS,
    },
    "moonshot-v1-32k": {
      context_window: 32 * 1024,
      max_output_tokens: 32 * 1024,
      default_max_tokens: DEFAULT_MAX_TOKENS,
    },
    "moonshot-v1-8k": {
      context_window: 8 * 1024,
      max_output_tokens: 8 * 1024,
      default_max_tokens: 4096,
    },
  },
  "kimi-coding-plan": {
    "kimi-k2.7-code": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
    },
    "kimi-k2.7-code-highspeed": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
    },
    "kimi-k2.6": {
      context_window: 256 * 1024,
      max_output_tokens: 256 * 1024,
      default_max_tokens: 32 * 1024,
    },
  },
};

export function capabilitiesForProvider(providerId: string): Record<string, ModelCapabilityWire> {
  return { ...(MODEL_CAPABILITIES[providerId] ?? {}) };
}

export function capabilityForModel(
  providerId: string,
  modelId: string,
): ModelCapabilityWire | undefined {
  return MODEL_CAPABILITIES[providerId]?.[modelId];
}

export function normalizeAdvancedWithModelCapability(
  advanced: ModelProfileAdvanced | undefined,
  providerId: string,
  modelId: string,
): ModelProfileAdvanced {
  const capability = capabilityForModel(providerId, modelId);
  const maxTokens =
    capability?.max_output_tokens !== undefined &&
    advanced?.maxTokens === capability.default_max_tokens
      ? capability.max_output_tokens
      : advanced?.maxTokens ?? capability?.max_output_tokens ?? capability?.default_max_tokens;

  return {
    ...advanced,
    maxTokens: maxTokens ?? DEFAULT_MAX_TOKENS,
    contextWindow: advanced?.contextWindow ?? capability?.context_window ?? DEFAULT_CONTEXT_WINDOW,
  };
}

export function withModelCapabilities(provider: ModelProviderWire): ModelProviderWire {
  const modelCapabilities = capabilitiesForProvider(provider.id);
  return Object.keys(modelCapabilities).length > 0
    ? { ...provider, model_capabilities: modelCapabilities }
    : provider;
}
