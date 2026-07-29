import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import type {
  AgentPlanAccountSaveRequest,
  AgentPlanAccountWire,
  ModelReasoningCapabilityTestResult,
  ModelCredentialReference,
  ModelProfileAdvanced,
  ModelProfileBalanceWire,
  ModelProfileSaveRequest,
  ModelProfileTestResult,
  ModelProviderWire,
  RuntimeModelProfileSummaryWire,
  ModelProfileRunConfig,
  ReasoningEffortCapabilityWire,
  ReasoningEffortLevel,
} from "../shared/runtime.js";
import { VOLCENGINE_AGENT_PLAN_ACCOUNT_ID } from "../shared/runtime.js";
import {
  capabilityForModel,
  normalizeAdvancedWithModelCapability,
  reasoningEffortCapability,
  resolveReasoningEffort,
  setCapabilityOverrides,
  validateReasoningEffort,
  withModelCapabilities,
} from "./model-capabilities.js";
import {
  capabilityEndpointContract,
  fetchProviderCapabilities,
  loadCapabilityOverrides,
  normalizeCapabilityBaseURL,
  parseProviderCapabilities,
  saveCapabilityOverrides,
  type CapabilityResolution,
  type ScopedCapabilityOverrides,
} from "./model-capability-sync.js";
import {
  fetchDeepSeekBalance,
  fetchKimiCodingBalance,
  BalanceFetchError,
} from "./model-balance-fetchers.js";
import {
  AwakenEnvStore,
  AwakenEnvStoreError,
} from "./awaken-env-store.js";
import { AgentPlanAccountStore } from "./agent-plan-account-store.js";

const MODEL_PROFILES_FILE = "models.json";
const DUPLICATE_MODEL_PROFILE_ERROR = "模型配置重复，添加失败";
const CAPABILITY_SYNC_TIMEOUT_MS = 8_000;
const BALANCE_FETCH_TIMEOUT_MS = 8_000;
const AGENT_PLAN_EFFORT_PROBE_TIMEOUT_MS = 8_000;

export const BUILTIN_MODEL_PROVIDERS: ModelProviderWire[] = [
  {
    id: "kimi-coding-plan",
    display_name: "Kimi Coding Plan",
    base_url: "https://api.kimi.com/coding/v1",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "KIMI_CODE_API_KEY",
    default_model: "k3",
    models: ["k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"],
    docs_url: "https://www.kimi.com/code/docs/kimi-code/models.html",
    api_docs_url: "https://www.kimi.com/code/console",
    access_type: "coding_plan",
    coding_preset: true,
  },
  {
    id: "volcengine",
    display_name: "火山 Coding Plan",
    base_url: "https://ark.cn-beijing.volces.com/api/coding/v3",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "ARK_API_KEY",
    default_model: "ark-code-latest",
    models: [
      "ark-code-latest",
      "doubao-seed-code",
      "doubao-seed-2.0-code",
      "doubao-seed-2.0-pro",
      "doubao-seed-2.0-lite",
      "minimax-m2.5",
      "minimax-m2.7",
      "minimax-m3",
      "glm-4.7",
      "glm-5.2",
      "glm-latest",
      "deepseek-v3.2",
      "deepseek-v4-flash",
      "deepseek-v4-pro",
      "kimi-k2.5",
      "kimi-k2.6",
      "kimi-k2.7-code",
    ],
    docs_url: "https://www.volcengine.com/docs/82379/2188958",
    api_docs_url: "https://www.volcengine.com/docs/82379/1541594",
    access_type: "coding_plan",
    coding_preset: true,
    allow_custom_model: true,
  },
  {
    id: "qwen-token-plan",
    display_name: "Qwen Token Plan",
    base_url: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "DASHSCOPE_TOKEN_PLAN_API_KEY",
    default_model: "qwen3.8-max-preview",
    models: ["qwen3.8-max-preview"],
    docs_url: "https://help.aliyun.com/zh/model-studio/token-plan",
    api_docs_url: "https://bailian.console.aliyun.com/",
    access_type: "coding_plan",
    coding_preset: true,
  },
  {
    id: "kimi-cn",
    display_name: "Kimi",
    base_url: "https://api.moonshot.cn/v1",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "MOONSHOT_API_KEY",
    default_model: "kimi-k3",
    models: ["kimi-k3"],
    docs_url: "https://platform.kimi.com/docs",
    api_docs_url: "https://platform.kimi.com/console/api-keys",
    access_type: "api",
  },
  {
    id: "minimax",
    display_name: "MiniMax",
    base_url: "https://api.minimax.io/v1",
    protocol: "minimax_chat",
    auth_scheme: "bearer",
    key_env_name: "MINIMAX_API_KEY",
    default_model: "MiniMax-M3",
    models: ["MiniMax-M3"],
    docs_url: "https://www.minimax.io/models/text/m3",
    api_docs_url: "https://platform.minimax.io/user-center/basic-information/interface-key",
    access_type: "api",
  },
  {
    id: "glm",
    display_name: "GLM",
    base_url: "https://open.bigmodel.cn/api/paas/v4",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "ZHIPU_API_KEY",
    default_model: "glm-5.2",
    models: ["glm-5.2"],
    docs_url: "https://docs.bigmodel.cn/cn/guide/models/text/glm-5.2",
    api_docs_url: "https://open.bigmodel.cn/usercenter/apikeys",
    access_type: "api",
  },
  {
    id: "qwen",
    display_name: "Qwen",
    base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "DASHSCOPE_API_KEY",
    default_model: "qwen3.7-max",
    models: ["qwen3.7-max"],
    docs_url: "https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions",
    api_docs_url: "https://bailian.console.aliyun.com/",
    access_type: "api",
    allow_custom_model: true,
  },
  {
    id: "doubao",
    display_name: "Doubao Ark",
    base_url: "https://ark.cn-beijing.volces.com/api/v3",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "ARK_API_KEY",
    default_model: "doubao-seed-2.1-pro",
    models: ["doubao-seed-2.1-pro"],
    docs_url: "https://www.volcengine.com/product/doubao",
    api_docs_url: "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey",
    access_type: "api",
    endpoint_model: true,
  },
  {
    id: "deepseek",
    display_name: "DeepSeek",
    base_url: "https://api.deepseek.com",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "DEEPSEEK_API_KEY",
    default_model: "deepseek-v4-pro",
    models: ["deepseek-v4-pro", "deepseek-v4-flash"],
    docs_url: "https://api-docs.deepseek.com/",
    api_docs_url: "https://platform.deepseek.com/api_keys",
    access_type: "api",
  },
  {
    id: "volcengine-agent-plan",
    display_name: "火山 Agent Plan",
    base_url: "https://ark.cn-beijing.volces.com/api/plan/v3",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "AWAKEN_AGENT_PLAN_API_KEY",
    default_model: "ark-code-latest",
    models: ["ark-code-latest"],
    docs_url: "https://www.volcengine.com/docs/82379",
    api_docs_url: "https://console.volcengine.com/ark/region:ark+cn-beijing/apiKey",
    access_type: "agent_plan",
    allow_custom_model: true,
  },
];

type StoredModelProfile = {
  id: string;
  providerId: string;
  displayName: string;
  modelId: string;
  baseURL: string;
  protocol: string;
  authScheme: string;
  enabled: boolean;
  isDefault: boolean;
  advanced?: ModelProfileAdvanced;
  keyLast4?: string;
  credentialRevision?: number;
  credentialReference?: ModelCredentialReference;
};

type StoreFile = {
  version: 1 | 2;
  profiles: StoredModelProfile[];
};

export interface ModelProfileKeyStore {
  getKey(profileId: string): Promise<string>;
  setKey(profileId: string, value: string): Promise<void>;
  deleteKey(profileId: string): Promise<void>;
}

function providerById(providerId: string): ModelProviderWire | undefined {
  return BUILTIN_MODEL_PROVIDERS.find((provider) => provider.id === providerId);
}

function trimTrailingSlash(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

function normalizeProviderModelId(provider: ModelProviderWire | undefined, modelId: string): string {
  const trimmed = modelId.trim();
  if (!provider) return trimmed;
  if (provider.endpoint_model) {
    return provider.models.includes(trimmed) ? trimmed : provider.default_model;
  }
  if (provider.allow_custom_model) return trimmed || provider.default_model;
  if (trimmed && provider.models.includes(trimmed)) return trimmed;
  return provider.default_model;
}

function normalizeProviderBaseURL(
  provider: ModelProviderWire | undefined,
  baseURL: string | undefined,
): string {
  const normalized = trimTrailingSlash(baseURL || provider?.base_url || "");
  if (provider?.id === "kimi-coding-plan" && normalized === "https://api.kimi.com/coding") {
    return "https://api.kimi.com/coding/v1";
  }
  if (provider?.id === "volcengine" && normalized === "https://ark.cn-beijing.volces.com/api/v3") {
    return "https://ark.cn-beijing.volces.com/api/coding/v3";
  }
  if (provider?.id === "volcengine-agent-plan") return provider.base_url;
  return normalized;
}

function capabilityResolutionForIdentity(
  provider: ModelProviderWire,
  profileId: string,
  credentialRevision: number,
  baseURL: string,
  protocol: string,
  selector: string,
): CapabilityResolution {
  return {
    scope: {
      provider_id: provider.id,
      normalized_base_url: normalizeCapabilityBaseURL(baseURL),
      protocol_family: protocol.trim().toLowerCase(),
      profile_id: profileId.trim(),
      credential_revision: Math.max(1, Math.floor(credentialRevision)),
      selector_kind: provider.endpoint_model ? "endpoint" : "model",
      endpoint_contract_id: capabilityEndpointContract(protocol),
    },
    selector: selector.trim(),
  };
}

const BALANCE_SUPPORTED_PROVIDERS = new Set<string>(["deepseek", "kimi-coding-plan"]);

function isBalanceSupportedProvider(providerId: string): boolean {
  return BALANCE_SUPPORTED_PROVIDERS.has(providerId);
}

async function fetchBalanceForProvider(
  providerId: string,
  baseURL: string,
  apiKey: string,
  signal: AbortSignal,
): Promise<ModelProfileBalanceWire["detail"]> {
  switch (providerId) {
    case "deepseek":
      return await fetchDeepSeekBalance(baseURL, apiKey, signal);
    case "kimi-coding-plan":
      return await fetchKimiCodingBalance(baseURL, apiKey, signal);
    default:
      throw new BalanceFetchError("暂不支持查询该供应商余额");
  }
}

function combineAbortSignals(...signals: AbortSignal[]): AbortSignal {
  const controller = new AbortController();
  const cleanup = (): void => {
    for (const signal of signals) {
      signal.removeEventListener("abort", onAbort);
    }
  };
  const onAbort = (): void => {
    cleanup();
    controller.abort();
  };
  for (const signal of signals) {
    if (signal.aborted) {
      controller.abort();
      return controller.signal;
    }
    signal.addEventListener("abort", onAbort);
  }
  return controller.signal;
}

function maskKey(keyLast4?: string): string | undefined {
  if (!keyLast4) {
    return undefined;
  }
  return `•••• ${keyLast4}`;
}

function profileStatus(
  profile: StoredModelProfile,
  hasKey: boolean,
  observedState?: "unreadable",
): RuntimeModelProfileSummaryWire["profile_status"] {
  if (!profile.enabled) return "disabled";
  if (observedState) return observedState;
  if (!hasKey) return "missing_key";
  return "ready";
}

function providerTestStatusMessage(status: number, provider?: ModelProviderWire): string {
  if (provider?.id === "kimi-coding-plan") {
    switch (status) {
      case 401:
        return "连接测试失败：API key 无效，或当前会员套餐无权访问所选 Kimi Code 模型（K3 需 Moderato 及以上，高速版需 Allegretto 及以上）。";
      case 402:
        return "连接测试失败：Kimi Code 会员权益无法验证，请确认会员状态。";
      case 403:
        return "连接测试失败：Kimi Code 拒绝当前客户端或账号权限，请确认会员权益、额度和客户端支持状态。";
      case 404:
        return "连接测试失败：未找到所选 Kimi Code 模型，或账号无权访问该模型。";
    }
  }
  if (provider?.id === "volcengine") {
    switch (status) {
      case 404:
        return "连接测试失败：火山 Coding Plan 未找到该模型，或当前 API key 无权访问该模型。";
    }
  }
  switch (status) {
    case 401:
      return "连接测试失败：API key 无效或未授权。";
    case 403:
      return "连接测试失败：API key 无权访问该服务。";
    case 404:
      return "连接测试失败：Base URL 不支持 /models，请检查服务地址。";
    case 429:
      return "连接测试失败：请求被限流，请稍后重试。";
    default:
      if (status >= 500) {
        return `连接测试失败：服务端返回 HTTP ${status}。`;
      }
      return `连接测试失败：服务返回 HTTP ${status}。`;
  }
}

async function testModelEndpoint(
  provider: ModelProviderWire,
  baseURL: string,
  modelId: string,
  apiKey: string,
  signal: AbortSignal,
): Promise<Response> {
	const body = connectionTestBody(provider, modelId);
  const endpoint =
    provider.protocol === "minimax_chat"
      ? `${baseURL}/text/chatcompletion_v2`
      : `${baseURL}/chat/completions`;
  return fetch(endpoint, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
    },
    body,
    signal,
  });
}

function connectionTestBody(provider: ModelProviderWire, modelId: string): string {
  return JSON.stringify({
    model: modelId || provider.default_model,
    messages: [{ role: "user", content: "ping" }],
    ...(provider.id === "kimi-coding-plan" ? { temperature: 1 } : {}),
    max_tokens: 1,
    stream: false,
  });
}

type AgentPlanReasoningCandidate = {
  levels: readonly ReasoningEffortLevel[];
  defaultLevel: ReasoningEffortLevel;
  nativeByLevel: Readonly<Record<ReasoningEffortLevel, string>>;
};

function agentPlanReasoningCandidate(modelId: string): AgentPlanReasoningCandidate | undefined {
  switch (modelId) {
    case "kimi-k3":
      return {
        levels: ["low", "high", "max"],
        defaultLevel: "max",
        nativeByLevel: { low: "low", high: "high", max: "max" },
      };
    case "glm-5.2":
    case "deepseek-v4-pro":
      return {
        levels: ["high", "max"],
        defaultLevel: "high",
        nativeByLevel: { low: "low", high: "high", max: "max" },
      };
    case "qwen3.8-max-preview":
      return {
        levels: ["low", "high", "max"],
        defaultLevel: "max",
        nativeByLevel: { low: "low", high: "medium", max: "xhigh" },
      };
    case "doubao-seed-2.1-pro":
      return {
        levels: ["low", "high", "max"],
        defaultLevel: "high",
        nativeByLevel: { low: "low", high: "medium", max: "high" },
      };
    default:
      return undefined;
  }
}

function providerTestSuccessMessage(provider: ModelProviderWire): string {
  if (provider.id === "kimi-coding-plan") {
    return "连接测试通过：Kimi Code 已响应 chat/completions。";
  }
  if (provider.id === "volcengine") {
    return "连接测试通过：火山 Coding Plan 已响应 chat/completions。";
  }
  return "连接测试通过：服务已响应 chat/completions。";
}

type ConnectionUsage = {
  inputTokens?: number;
  cachedInputTokens?: number;
  uncachedInputTokens?: number;
  cacheWriteTokens?: number;
  outputTokens?: number;
  reasoningTokens?: number;
  totalTokens?: number;
  rawTypedUsage?: Record<string, number>;
};

function finiteUsage(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : undefined;
}

function connectionUsage(body: unknown): ConnectionUsage {
  if (!body || typeof body !== "object") return {};
  const usage = (body as { usage?: Record<string, unknown> }).usage;
  if (!usage || typeof usage !== "object") return {};
  const inputTokens = finiteUsage(usage.prompt_tokens ?? usage.input_tokens);
  const cachedInputTokens = finiteUsage(
    usage.cached_tokens ?? usage.prompt_cache_hit_tokens ?? usage.cache_read_input_tokens,
  );
  const cacheWriteTokens = finiteUsage(usage.cache_creation_input_tokens);
  const outputTokens = finiteUsage(usage.completion_tokens ?? usage.output_tokens);
  const completionDetails = usage.completion_tokens_details;
  const reasoningTokens = finiteUsage(
    usage.reasoning_tokens ??
      (completionDetails && typeof completionDetails === "object"
        ? (completionDetails as Record<string, unknown>).reasoning_tokens
        : undefined),
  );
  const totalTokens = finiteUsage(usage.total_tokens);
  const uncachedInputTokens =
    inputTokens !== undefined &&
    (cachedInputTokens !== undefined || cacheWriteTokens !== undefined) &&
    (cachedInputTokens ?? 0) + (cacheWriteTokens ?? 0) <= inputTokens
      ? inputTokens - (cachedInputTokens ?? 0) - (cacheWriteTokens ?? 0)
      : undefined;
  const rawTypedUsage = Object.fromEntries(
    Object.entries(usage).filter(([, value]) => finiteUsage(value) !== undefined),
  ) as Record<string, number>;
  return {
    inputTokens,
    cachedInputTokens,
    uncachedInputTokens,
    cacheWriteTokens,
    outputTokens,
    reasoningTokens,
    totalTokens,
    rawTypedUsage,
  };
}

function connectionPricing(
  providerId: string,
  baseURL: string,
  modelId: string,
  usage: ConnectionUsage,
): Record<string, unknown> {
  if (providerId !== "deepseek" || baseURL !== "https://api.deepseek.com") return {};
  const rates =
    modelId === "deepseek-v4-flash"
      ? { cached: 0.0028, uncached: 0.14, output: 0.28 }
      : modelId === "deepseek-v4-pro"
        ? { cached: 0.003625, uncached: 0.435, output: 0.87 }
        : undefined;
  if (!rates) return {};
  const pricingSnapshot = {
    snapshot_id: `deepseek-2026-07-19-${modelId.replace("deepseek-", "")}-usd`,
    provider_id: providerId,
    model_id: modelId,
    currency: "USD",
    unit_tokens: 1_000_000,
    cached_input_per_unit_usd: rates.cached,
    uncached_input_per_unit_usd: rates.uncached,
    output_per_unit_usd: rates.output,
    effective_date: "2026-07-19",
    source_url: "https://api-docs.deepseek.com/quick_start/pricing/",
  };
  const calculatedCostUSD =
    usage.cachedInputTokens !== undefined &&
    usage.uncachedInputTokens !== undefined &&
    usage.outputTokens !== undefined
      ? (usage.cachedInputTokens * rates.cached +
          usage.uncachedInputTokens * rates.uncached +
          usage.outputTokens * rates.output) /
        1_000_000
      : undefined;
  return {
    pricing_snapshot: pricingSnapshot,
    calculated_cost_usd: calculatedCostUSD,
    currency: "USD",
  };
}

function toSummary(
  profile: StoredModelProfile,
  hasKey: boolean,
  observedState?: "unreadable",
  credentialRevision?: number,
  resolution?: CapabilityResolution,
): RuntimeModelProfileSummaryWire {
  const provider = providerById(profile.providerId);
  const modelId = normalizeProviderModelId(provider, profile.modelId);
  const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
  const advanced = normalizeAdvancedWithModelCapability(
    profile.advanced,
    profile.providerId,
    modelId,
    resolution,
  );
  return {
    id: profile.id,
    provider_id: profile.providerId,
    display_name: profile.displayName,
    model_id: modelId,
    base_url: baseURL,
    protocol: profile.protocol,
    auth_scheme: profile.authScheme,
    has_key: hasKey,
    masked_key: hasKey ? maskKey(profile.keyLast4) : undefined,
    enabled: profile.enabled,
    is_default: profile.isDefault,
    profile_status: profileStatus(profile, hasKey, observedState),
    access_type: provider?.access_type ?? (provider?.coding_preset ? "coding_plan" : "api"),
    credential_reference:
      profile.credentialReference?.kind === "provider_account"
        ? {
            kind: "provider_account",
            provider_id: profile.credentialReference.providerId,
            account_id: profile.credentialReference.accountId,
          }
        : {
            kind: "profile_secret",
            profile_id: profile.id,
          },
    advanced: {
      temperature: advanced.temperature,
      max_tokens: advanced.maxTokens,
      endpoint_id: advanced.endpointId,
      prompt_cache_key: advanced.promptCacheKey,
      context_window: advanced.contextWindow,
      supports_vision: advanced.supportsVision,
      supports_native_pdf: advanced.supportsNativePDF,
      supports_video: advanced.supportsVideo,
      supports_tools: advanced.supportsTools,
      default_reasoning_effort: advanced.defaultReasoningEffort,
    },
    credential_revision: credentialRevision ?? profile.credentialRevision ?? 1,
    effective_capability: capabilityForModel(
      profile.providerId,
      modelId,
      resolution,
    ),
  };
}

function templateProfileForSave(
  store: StoreFile,
  providerId: string,
  templateProfileId: string | undefined,
): StoredModelProfile | undefined {
  if (!templateProfileId) return undefined;
  const templateProfile = store.profiles.find((profile) => profile.id === templateProfileId);
  if (!templateProfile || templateProfile.providerId !== providerId) {
    return undefined;
  }
  return templateProfile;
}

function storedProfileIdentityKey(profile: StoredModelProfile): string {
  const provider = providerById(profile.providerId);
  const modelId = normalizeProviderModelId(provider, profile.modelId);
  return `${profile.providerId}::${modelId}`;
}

function mergeStoredProfiles(
  preferred: StoredModelProfile,
  fallback: StoredModelProfile,
): StoredModelProfile {
  return {
    ...preferred,
    displayName: preferred.displayName || fallback.displayName,
    baseURL: preferred.baseURL || fallback.baseURL,
    protocol: preferred.protocol || fallback.protocol,
    authScheme: preferred.authScheme || fallback.authScheme,
    advanced: preferred.advanced ?? fallback.advanced,
    // Keys are addressed by profile ID in .env. Never transfer key metadata
    // from a duplicate record with a different ID.
    keyLast4: preferred.keyLast4,
    isDefault: preferred.isDefault || fallback.isDefault,
  };
}

function normalizeStoredProfile(profile: StoredModelProfile): StoredModelProfile {
  const keyLast4 = typeof profile.keyLast4 === "string" ? profile.keyLast4.slice(-4) : undefined;
  return {
    id: profile.id,
    providerId: profile.providerId,
    displayName: profile.displayName,
    modelId: profile.modelId,
    baseURL: profile.baseURL,
    protocol: profile.protocol,
    authScheme: profile.authScheme,
    enabled: profile.enabled !== false,
    isDefault: profile.isDefault === true,
    advanced: profile.advanced,
    keyLast4,
    credentialRevision:
      typeof profile.credentialRevision === "number" &&
      Number.isFinite(profile.credentialRevision) &&
      profile.credentialRevision >= 1
        ? Math.floor(profile.credentialRevision)
        : 1,
    credentialReference:
      profile.credentialReference?.kind === "provider_account"
        ? profile.credentialReference
        : { kind: "profile_secret", profileId: profile.id },
  };
}

function dedupeStoredProfiles(profiles: StoredModelProfile[]): StoredModelProfile[] {
  const dedupedByIdentity = new Map<string, StoredModelProfile>();
  const orderedProfiles: StoredModelProfile[] = [];

  for (let index = profiles.length - 1; index >= 0; index -= 1) {
    const profile = profiles[index];
    const identityKey = storedProfileIdentityKey(profile);
    const existing = dedupedByIdentity.get(identityKey);
    if (existing) {
      dedupedByIdentity.set(identityKey, mergeStoredProfiles(existing, profile));
      continue;
    }
    dedupedByIdentity.set(identityKey, profile);
    orderedProfiles.unshift(profile);
  }

  return orderedProfiles.map((profile) => dedupedByIdentity.get(storedProfileIdentityKey(profile)) ?? profile);
}

export class ModelProfileStore {
  private filePath: string;
  private cacheDir: string;
  private usageDir: string;
  private envStore: ModelProfileKeyStore;
  private agentPlanAccountStore: AgentPlanAccountStore;
  private agentPlanModelIds = ["ark-code-latest"];
  private observedCredentialStates = new Map<string, "unreadable">();
  private mutationQueue: Promise<void> = Promise.resolve();
  private capabilityOverrides: ScopedCapabilityOverrides[] = [];

  constructor(
    awakenHome: string,
    envStore: ModelProfileKeyStore = new AwakenEnvStore(awakenHome),
    agentPlanAccountStore: AgentPlanAccountStore = new AgentPlanAccountStore(awakenHome),
  ) {
    this.filePath = path.join(awakenHome, "config", MODEL_PROFILES_FILE);
    this.cacheDir = path.join(awakenHome, "cache");
    this.usageDir = path.join(awakenHome, "usage", "model-profiles");
    this.envStore = envStore;
    this.agentPlanAccountStore = agentPlanAccountStore;
  }

  async initialize(): Promise<void> {
    this.capabilityOverrides = await loadCapabilityOverrides(this.cacheDir);
    setCapabilityOverrides(this.capabilityOverrides);
    try {
      const account = await this.agentPlanAccountStore.getAccount();
      this.agentPlanModelIds = [...account.model_ids];
    } catch {
      this.agentPlanModelIds = ["ark-code-latest"];
    }
  }

  async syncCapabilities(): Promise<ScopedCapabilityOverrides[]> {
    const store = await this.readStore();
    const candidates: Array<{
      baseURL: string;
      apiKey: string;
      resolution: CapabilityResolution;
    }> = [];

    for (const profile of store.profiles) {
      if (!profile.enabled) continue;
      const provider = providerById(profile.providerId);
      if (!provider) continue;
      let credential: { apiKey: string; credentialRevision?: number };
      try {
        credential = await this.readProfileCredential(profile);
      } catch {
        continue;
      }
      if (!credential.apiKey) continue;
      const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
      const capabilityModelId = normalizeProviderModelId(provider, profile.modelId);
      const selector = provider.endpoint_model
        ? profile.advanced?.endpointId?.trim() || ""
        : capabilityModelId;
      if (!selector) continue;
      const credentialRevision =
        credential.credentialRevision ?? profile.credentialRevision ?? 1;
      candidates.push({
        baseURL,
        apiKey: credential.apiKey,
        resolution: capabilityResolutionForIdentity(
          provider,
          profile.id,
          credentialRevision,
          baseURL,
          profile.protocol || provider.protocol,
          selector,
        ),
      });
    }

    const fetched: ScopedCapabilityOverrides[] = [];
    await Promise.allSettled(
      candidates.map(async ({ baseURL, apiKey, resolution }) => {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), CAPABILITY_SYNC_TIMEOUT_MS);
        try {
          const overrides = await fetchProviderCapabilities(baseURL, apiKey, controller.signal);
          fetched.push({ scope: resolution.scope, models: overrides });
        } catch {
          // Ignore per-provider fetch failures; keep existing overrides.
        } finally {
          clearTimeout(timeout);
        }
      }),
    );

    this.capabilityOverrides = await saveCapabilityOverrides(
      this.cacheDir,
      fetched,
    );
    setCapabilityOverrides(this.capabilityOverrides);
    return this.capabilityOverrides;
  }

  listProviders(): ModelProviderWire[] {
    return BUILTIN_MODEL_PROVIDERS.map((provider) =>
      withModelCapabilities({
        ...provider,
        models:
          provider.id === "volcengine-agent-plan"
            ? [...this.agentPlanModelIds]
            : [...provider.models],
      }),
    );
  }

  async getAgentPlanAccount(): Promise<AgentPlanAccountWire> {
    return this.agentPlanAccountStore.getAccount();
  }

  async saveAgentPlanAccount(request: AgentPlanAccountSaveRequest): Promise<AgentPlanAccountWire> {
    const account = await this.agentPlanAccountStore.saveAccount(request);
    this.agentPlanModelIds = [...account.model_ids];
    return account;
  }

  async refreshAgentPlanModels(): Promise<AgentPlanAccountWire> {
    const account = await this.agentPlanAccountStore.refreshModelCatalog();
    this.agentPlanModelIds = [...account.model_ids];
    return account;
  }

  async deleteAgentPlanAccount(): Promise<void> {
    const store = await this.readStore();
    const referenced = store.profiles.some(
      (profile) => profile.credentialReference?.kind === "provider_account",
    );
    await this.agentPlanAccountStore.deleteAccount(referenced);
    this.agentPlanModelIds = ["ark-code-latest"];
  }

  async listProfiles(): Promise<RuntimeModelProfileSummaryWire[]> {
    const store = await this.readStore();
    return Promise.all(store.profiles.map((profile) => this.toSummary(profile)));
  }

  async saveProfile(request: ModelProfileSaveRequest): Promise<RuntimeModelProfileSummaryWire> {
    const provider = providerById(request.providerId);
    if (!provider) {
      throw new Error("未知模型供应商");
    }

    return this.withStoreMutation(() =>
      this.saveProfileWithProvider(request, provider),
    );
  }

  private async saveProfileWithProvider(
    request: ModelProfileSaveRequest,
    provider: ModelProviderWire,
  ): Promise<RuntimeModelProfileSummaryWire> {
    const store = await this.readStore();
    const existingIndex = request.id
      ? store.profiles.findIndex((profile) => profile.id === request.id)
      : -1;
    const modelId = normalizeProviderModelId(provider, request.modelId);
    const baseURL = normalizeProviderBaseURL(provider, request.baseURL);
    const existing = existingIndex >= 0 ? store.profiles[existingIndex] : undefined;
    const templateProfile = templateProfileForSave(
      store,
      provider.id,
      request.templateProfileId,
    );
    const duplicateProfile = store.profiles.find(
      (profile) =>
        profile.id !== existing?.id &&
        profile.providerId === provider.id &&
        normalizeProviderModelId(provider, profile.modelId) === modelId,
    );
    if (duplicateProfile) {
      throw new Error(DUPLICATE_MODEL_PROFILE_ERROR);
    }
    const sourceAdvanced =
      request.advanced ?? existing?.advanced ?? templateProfile?.advanced;
    let capabilityCredentialRevision = existing?.credentialRevision ?? 1;
    if (provider.id === "volcengine-agent-plan") {
      try {
        capabilityCredentialRevision = (
          await this.agentPlanAccountStore.getAccount()
        ).credential_revision;
      } catch {
        capabilityCredentialRevision = 1;
      }
    }
    const capabilityResolution = existing
      ? capabilityResolutionForIdentity(
          provider,
          existing.id,
          capabilityCredentialRevision,
          baseURL,
          request.protocol?.trim() || existing.protocol || provider.protocol,
          provider.endpoint_model
            ? sourceAdvanced?.endpointId?.trim() || ""
            : modelId,
        )
      : undefined;
    const advanced = normalizeAdvancedWithModelCapability(
      sourceAdvanced,
      provider.id,
      modelId,
      capabilityResolution,
    );
    validateReasoningEffort(
      provider.id,
      modelId,
      request.advanced?.defaultReasoningEffort,
      capabilityResolution,
    );
    if (provider.endpoint_model && !/^ep-[A-Za-z0-9_-]+$/.test(advanced.endpointId?.trim() ?? "")) {
      throw new Error("Doubao Ark 需要填写 ep-* 推理接入点。");
    }
    const profile: StoredModelProfile = {
      id: existing?.id ?? randomUUID(),
      providerId: provider.id,
      displayName: request.displayName?.trim() || provider.display_name,
      modelId,
      baseURL,
      protocol: request.protocol?.trim() || provider.protocol,
      authScheme: request.authScheme?.trim() || provider.auth_scheme,
      enabled: request.enabled ?? existing?.enabled ?? true,
      isDefault: request.isDefault ?? existing?.isDefault ?? store.profiles.length === 0,
      advanced,
      keyLast4: existing?.keyLast4,
      credentialRevision: existing?.credentialRevision ?? 1,
      credentialReference:
        provider.id === "volcengine-agent-plan"
          ? {
              kind: "provider_account",
              providerId: "volcengine-agent-plan",
              accountId: VOLCENGINE_AGENT_PLAN_ACCOUNT_ID,
            }
          : { kind: "profile_secret", profileId: existing?.id ?? "" },
    };
    if (profile.credentialReference?.kind === "profile_secret") {
      profile.credentialReference = { kind: "profile_secret", profileId: profile.id };
    }

    const agentPlanKeyToPersist =
      provider.id === "volcengine-agent-plan" && request.apiKey?.trim()
        ? request.apiKey.trim()
        : undefined;
    let apiKeyToPersist: string | undefined;
    if (
      provider.id !== "volcengine-agent-plan" &&
      request.apiKey !== undefined &&
      request.apiKey.trim() !== ""
    ) {
      apiKeyToPersist = request.apiKey.trim();
    } else if (!existing && templateProfile) {
      const templateKey = await this.readProfileKey(templateProfile);
      if (templateKey) apiKeyToPersist = templateKey;
    }
    if (apiKeyToPersist !== undefined && provider.id !== "volcengine-agent-plan") {
      profile.keyLast4 = apiKeyToPersist.slice(-4);
    }

    let previousKeyForMutation: string | undefined;
    if (apiKeyToPersist !== undefined) {
      previousKeyForMutation = await this.envStore.getKey(profile.id);
      if (
        existing &&
        previousKeyForMutation !== "" &&
        previousKeyForMutation !== apiKeyToPersist
      ) {
        profile.credentialRevision = (existing.credentialRevision ?? 1) + 1;
      }
    }
    if (!profile.enabled) {
      profile.isDefault = false;
    }
    if (existingIndex >= 0) {
      store.profiles[existingIndex] = profile;
    } else {
      store.profiles.push(profile);
    }

    this.ensureSingleDefault(store, profile.id);
    if (agentPlanKeyToPersist !== undefined) {
      const account = await this.agentPlanAccountStore.saveAccountWithCompensation(
        { apiKey: agentPlanKeyToPersist },
        () => this.writeStore(store),
      );
      this.agentPlanModelIds = [...account.model_ids];
    } else if (apiKeyToPersist !== undefined) {
      await this.envStore.setKey(profile.id, apiKeyToPersist);
      try {
        await this.writeStore(store);
      } catch (error) {
        await this.restoreProfileKey(profile.id, previousKeyForMutation ?? "");
        throw error;
      }
      this.observedCredentialStates.delete(profile.id);
    } else {
      await this.writeStore(store);
    }
    return this.toSummary(profile);
  }

  async deleteProfile(profileId: string): Promise<void> {
    await this.withStoreMutation(async () => {
      const store = await this.readStore();
      const previousDefault = store.profiles.find((profile) => profile.id === profileId)?.isDefault;
      store.profiles = store.profiles.filter((profile) => profile.id !== profileId);
      if (previousDefault && store.profiles.length > 0) {
        store.profiles[0].isDefault = true;
      }
      const previousKey = await this.envStore.getKey(profileId);
      await this.envStore.deleteKey(profileId);
      try {
        await this.writeStore(store);
      } catch (error) {
        await this.restoreProfileKey(profileId, previousKey);
        throw error;
      }
      this.observedCredentialStates.delete(profileId);
    });
  }

  async setDefaultProfile(profileId: string): Promise<RuntimeModelProfileSummaryWire> {
    return this.withStoreMutation(async () => {
      const store = await this.readStore();
      const profile = store.profiles.find((entry) => entry.id === profileId);
      if (!profile) {
        throw new Error("Model profile not found");
      }
      profile.enabled = true;
      for (const entry of store.profiles) {
        entry.isDefault = entry.id === profileId;
      }
      await this.writeStore(store);
      return this.toSummary(profile);
    });
  }

  async resolveDefaultRunProfile(): Promise<ModelProfileRunConfig | undefined> {
    return this.resolveRunProfile();
  }

  async resolveRunProfile(
    profileId?: string,
    requestedEffort?: import("../shared/runtime.js").ReasoningEffortLevel,
  ): Promise<ModelProfileRunConfig | undefined> {
    const store = await this.readStore();
    const profile =
      store.profiles.find((entry) => entry.id === profileId && entry.enabled) ??
      store.profiles.find((entry) => entry.isDefault && entry.enabled) ??
      store.profiles.find((entry) => entry.enabled);
    if (!profile) return undefined;
    const provider = providerById(profile.providerId);
    const capabilityModelId = normalizeProviderModelId(provider, profile.modelId);
    const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
    const credential = await this.readProfileCredential(profile);
    const apiKey = credential.apiKey;
    if (!apiKey) {
      throw new Error("请先在设置 > 模型中配置 API key。");
    }
    const credentialRevision =
      credential.credentialRevision ?? profile.credentialRevision ?? 1;
    const endpointSelector = provider?.endpoint_model
      ? profile.advanced?.endpointId?.trim() || ""
      : undefined;
    const modelId = endpointSelector || capabilityModelId;
    const capabilityResolution = provider
      ? capabilityResolutionForIdentity(
          provider,
          profile.id,
          credentialRevision,
          baseURL,
          profile.protocol || provider.protocol,
          modelId,
        )
      : undefined;
    const advanced = normalizeAdvancedWithModelCapability(
      profile.advanced,
      profile.providerId,
      capabilityModelId,
      capabilityResolution,
    );
    return {
      profileId: profile.id,
      providerId: profile.providerId,
      displayName: profile.displayName,
      modelId,
      capabilityModelId,
      endpointSelector,
      modelSelectorKind: provider?.endpoint_model ? "endpoint" : "model",
      baseURL,
      protocol: profile.protocol,
      authScheme: profile.authScheme,
      apiKey,
      temperature: advanced.temperature,
      maxTokens: advanced.maxTokens,
      contextWindow: advanced.contextWindow,
      promptCacheKey: advanced.promptCacheKey,
      credentialRevision,
      supportsVision: advanced.supportsVision,
      supportsNativePDF: advanced.supportsNativePDF,
      supportsVideo: advanced.supportsVideo,
      supportsTools: advanced.supportsTools,
      reasoningEffort: resolveReasoningEffort(
        profile.providerId,
        capabilityModelId,
        requestedEffort,
        advanced.defaultReasoningEffort,
        capabilityResolution,
      ),
    };
  }

  async reasoningCapabilityForProfile(
    profileId: string,
  ): Promise<ReasoningEffortCapabilityWire | undefined> {
    const store = await this.readStore();
    const profile = store.profiles.find(
      (entry) => entry.id === profileId && entry.enabled,
    );
    if (!profile) return undefined;
    const provider = providerById(profile.providerId);
    if (!provider) return undefined;
    const capabilityModelId = normalizeProviderModelId(provider, profile.modelId);
    const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
    const credential = await this.readProfileCredential(profile);
    const credentialRevision =
      credential.credentialRevision ?? profile.credentialRevision ?? 1;
    const selector = provider.endpoint_model
      ? profile.advanced?.endpointId?.trim() || ""
      : capabilityModelId;
    if (!selector) return undefined;
    return reasoningEffortCapability(
      profile.providerId,
      capabilityModelId,
      capabilityResolutionForIdentity(
        provider,
        profile.id,
        credentialRevision,
        baseURL,
        profile.protocol || provider.protocol,
        selector,
      ),
    );
  }

  async defaultProfileSummary(): Promise<RuntimeModelProfileSummaryWire | undefined> {
    const store = await this.readStore();
    const profile =
      store.profiles.find((entry) => entry.isDefault) ??
      store.profiles.find((entry) => entry.enabled);
    return profile ? await this.toSummary(profile) : undefined;
  }

  private connectionUsageScope(
    request: ModelProfileSaveRequest,
    providerId: string,
    baseURL: string,
    modelId: string,
  ): string {
    const explicit = request.id?.trim() || request.templateProfileId?.trim();
    if (explicit) return explicit.replace(/[^A-Za-z0-9._-]/g, "_");
    return `profile_${createHash("sha256")
      .update(`${providerId}\0${baseURL}\0${modelId}`)
      .digest("hex")
      .slice(0, 24)}`;
  }

  private async appendConnectionUsage(
    scope: string,
    record: Record<string, unknown>,
  ): Promise<void> {
    await fs.mkdir(this.usageDir, { recursive: true, mode: 0o700 });
    await fs.chmod(this.usageDir, 0o700);
    const target = path.join(this.usageDir, `${scope}.jsonl`);
    const handle = await fs.open(target, "a", 0o600);
    try {
      await handle.chmod(0o600);
      await handle.write(`${JSON.stringify(record)}\n`);
      await handle.sync();
    } finally {
      await handle.close();
    }
  }

  async testProfile(request: ModelProfileSaveRequest): Promise<ModelProfileTestResult> {
    const provider = providerById(request.providerId);
    if (!provider) {
      return { ok: false, message: "连接测试失败：未知模型供应商。" };
    }
    const agentPlanKey =
      provider.id === "volcengine-agent-plan"
        ? (await this.agentPlanAccountStore.resolveKey()).apiKey
        : "";
    const apiKey =
      request.apiKey?.trim() ||
      agentPlanKey ||
      (request.id
        ? await this.apiKeyForProfile(request.id)
        : request.templateProfileId
          ? await this.apiKeyForProfile(request.templateProfileId)
          : "");
    if (!apiKey) {
      return { ok: false, message: "连接测试失败：请先填写 API key。" };
    }
    const baseURL = normalizeProviderBaseURL(provider, request.baseURL);
    const modelId = provider.endpoint_model
      ? request.advanced?.endpointId?.trim() || ""
      : normalizeProviderModelId(provider, request.modelId);
    const requestId = `connection_${randomUUID().replaceAll("-", "")}`;
    const requestBody = connectionTestBody(provider, modelId);
    const requestShapeDigest = createHash("sha256").update(requestBody).digest("hex");
    const usageScope = this.connectionUsageScope(request, provider.id, baseURL, modelId);
    const startedAt = Date.now();
    await this.appendConnectionUsage(usageScope, {
      schema_version: 1,
      kind: "model_request",
      request_id: requestId,
      purpose: "connection_test",
      provider_id: provider.id,
      model_id: modelId,
      protocol_family: provider.protocol,
      request_shape_digest: requestShapeDigest,
      max_output_tokens: 1,
      created_at: new Date(startedAt).toISOString(),
    });
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 8000);
    try {
      const response = await testModelEndpoint(provider, baseURL, modelId, apiKey, controller.signal);
      let parsedBody: unknown;
      try {
        parsedBody = await response.json();
      } catch {
        parsedBody = undefined;
      }
      const usage = connectionUsage(parsedBody);
      const pricing = connectionPricing(provider.id, baseURL, modelId, usage);
      await this.appendConnectionUsage(usageScope, {
        schema_version: 1,
        kind: "model_usage",
        request_id: requestId,
        purpose: "connection_test",
        provider_id: provider.id,
        model_id: modelId,
        protocol_family: provider.protocol,
        input_tokens: usage.inputTokens,
        cached_input_tokens: usage.cachedInputTokens,
        uncached_input_tokens: usage.uncachedInputTokens,
        cache_write_tokens: usage.cacheWriteTokens,
        output_tokens: usage.outputTokens,
        reasoning_tokens: usage.reasoningTokens,
        total_tokens: usage.totalTokens,
        raw_typed_usage: usage.rawTypedUsage,
        ...pricing,
        usage_complete: usage.inputTokens !== undefined || usage.outputTokens !== undefined,
        provider_request_id: response.headers?.get?.("x-request-id") || undefined,
        latency_ms: Date.now() - startedAt,
        status: response.ok ? "succeeded" : "failed",
        error_code: response.ok ? undefined : `http_${response.status}`,
        completed_at: new Date().toISOString(),
      });
      if (response.ok) {
        if (
          parsedBody !== undefined &&
          provider.id !== "kimi-coding-plan" &&
          provider.id !== "volcengine"
        ) {
          try {
            const overrides = parseProviderCapabilities(parsedBody);
            const resolution = await this.capabilityResolutionForRequest(
              request,
              provider,
              baseURL,
              modelId,
            );
            if (resolution) {
              this.capabilityOverrides = await saveCapabilityOverrides(
                this.cacheDir,
                [{ scope: resolution.scope, models: overrides }],
              );
              setCapabilityOverrides(this.capabilityOverrides);
            }
          } catch {
            // A successful connection test does not depend on capability parsing.
          }
        }
        return { ok: true, message: providerTestSuccessMessage(provider) };
      }
      return { ok: false, message: providerTestStatusMessage(response.status, provider) };
    } catch (error) {
      await this.appendConnectionUsage(usageScope, {
        schema_version: 1,
        kind: "model_usage",
        request_id: requestId,
        purpose: "connection_test",
        provider_id: provider.id,
        model_id: modelId,
        protocol_family: provider.protocol,
        usage_complete: false,
        latency_ms: Date.now() - startedAt,
        status: error instanceof Error && error.name === "AbortError" ? "cancelled" : "failed",
        error_code: error instanceof Error && error.name === "AbortError" ? "timeout" : "transport_error",
        completed_at: new Date().toISOString(),
      });
      if (error instanceof Error && error.name === "AbortError") {
        return { ok: false, message: "连接测试超时：8 秒内未收到服务响应。" };
      }
      return { ok: false, message: "连接测试失败：无法连接到服务，请检查 Base URL 或网络。" };
    } finally {
      clearTimeout(timeout);
    }
  }

  async testAgentPlanReasoningCapability(
    request: ModelProfileSaveRequest,
  ): Promise<ModelReasoningCapabilityTestResult> {
    if (request.providerId !== "volcengine-agent-plan") {
      return { ok: false, message: "推理强度验证仅用于 Agent Plan 精确模型 ID。" };
    }
    const modelId = request.modelId.trim();
    const candidate = agentPlanReasoningCandidate(modelId);
    if (!candidate || modelId === "ark-code-latest") {
      return { ok: false, message: "该模型 ID 未命中已知能力矩阵，推理强度保持 hidden。" };
    }
    const apiKey = request.apiKey?.trim() || (await this.agentPlanAccountStore.resolveKey()).apiKey;
    if (!apiKey) return { ok: false, message: "请先保存 Agent Plan API key。" };
    const controller = new AbortController();
    const timeout = setTimeout(
      () => controller.abort(),
      AGENT_PLAN_EFFORT_PROBE_TIMEOUT_MS,
    );
    try {
      for (const level of candidate.levels) {
        const response = await fetch(
          "https://ark.cn-beijing.volces.com/api/plan/v3/chat/completions",
          {
            method: "POST",
            headers: { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" },
            body: JSON.stringify({
              model: modelId,
              messages: [{ role: "user", content: "ping" }],
              reasoning_effort: candidate.nativeByLevel[level],
              max_tokens: 1,
              stream: false,
            }),
            signal: controller.signal,
          },
        );
        if (!response.ok) {
          return {
            ok: false,
            message: `套餐端点未接受 ${level} 档 effort 参数（HTTP ${response.status}），完整矩阵继续隐藏。`,
          };
        }
      }
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        return {
          ok: false,
          message: "Agent Plan 推理强度验证超时：完整矩阵继续隐藏。",
        };
      }
      return {
        ok: false,
        message: "Agent Plan 推理强度验证失败：完整矩阵继续隐藏。",
      };
    } finally {
      clearTimeout(timeout);
    }
    const reasoningEffort = { levels: [...candidate.levels], defaultLevel: candidate.defaultLevel };
    const resolution = await this.capabilityResolutionForRequest(
      request,
      providerById("volcengine-agent-plan")!,
      "https://ark.cn-beijing.volces.com/api/plan/v3",
      modelId,
    );
    if (resolution) {
      this.capabilityOverrides = await saveCapabilityOverrides(
        this.cacheDir,
        [{
          scope: resolution.scope,
          models: {
            [modelId]: {
              reasoning_effort: {
                levels: [...candidate.levels],
                default_level: candidate.defaultLevel,
              },
            },
          },
        }],
      );
      setCapabilityOverrides(this.capabilityOverrides);
    }
    return { ok: true, message: "Agent Plan 推理强度验证通过；本次验证可能产生少量模型消耗。", reasoningEffort };
  }

  async fetchBalances(signal?: AbortSignal): Promise<ModelProfileBalanceWire[]> {
    const store = await this.readStore();
    const candidateProfiles = store.profiles.filter((profile) => profile.enabled);

    const providerKeys = new Map<string, string>();
    const providerKeyProfileIds = new Map<string, string>();
    const providerBaseURLs = new Map<string, string>();
    const fetchedDetails = new Map<string, ModelProfileBalanceWire["detail"] | null>();
    const fetchErrors = new Map<string, string>();
    const credentialIssues = new Map<string, "unreadable">();

    for (const profile of candidateProfiles) {
      if (providerKeys.has(profile.providerId)) continue;
      const provider = providerById(profile.providerId);
      if (!provider) continue;
      if (!isBalanceSupportedProvider(profile.providerId)) continue;
      let apiKey = "";
      try {
        apiKey = await this.readProfileKey(profile);
      } catch (error) {
        if (error instanceof AwakenEnvStoreError) {
          credentialIssues.set(profile.providerId, "unreadable");
        }
        fetchErrors.set(
          profile.providerId,
          error instanceof Error
            ? error.message
            : `模型配置「${profile.displayName}」的 API key 无法读取。`,
        );
        continue;
      }
      if (!apiKey) continue;
      providerKeys.set(profile.providerId, apiKey);
      providerKeyProfileIds.set(profile.providerId, profile.id);
      providerBaseURLs.set(
        profile.providerId,
        normalizeProviderBaseURL(provider, profile.baseURL),
      );
    }

    await Promise.allSettled(
      Array.from(providerKeys.entries()).map(async ([providerId, apiKey]) => {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), BALANCE_FETCH_TIMEOUT_MS);
        const combinedSignal = signal
          ? combineAbortSignals(signal, controller.signal)
          : controller.signal;
        try {
          const baseURL = providerBaseURLs.get(providerId) ?? "";
          const detail = await fetchBalanceForProvider(
            providerId,
            baseURL,
            apiKey,
            combinedSignal,
          );
          fetchedDetails.set(providerId, detail);
        } catch (error) {
          const message =
            error instanceof BalanceFetchError
              ? error.message
              : error instanceof Error && error.name === "AbortError"
                ? "查询余额超时"
                : "查询余额失败";
          fetchErrors.set(providerId, message);
        } finally {
          clearTimeout(timeout);
        }
      }),
    );

    // Balance is account/provider-scoped: one list entry per provider regardless of
    // how many enabled profiles share that vendor.
    const seenProviders = new Set<string>();
    const balances: ModelProfileBalanceWire[] = [];

    for (const profile of candidateProfiles) {
      if (seenProviders.has(profile.providerId)) continue;
      seenProviders.add(profile.providerId);

      const provider = providerById(profile.providerId);
      const representativeId = providerKeyProfileIds.get(profile.providerId);
      const representative =
        (representativeId
          ? candidateProfiles.find((entry) => entry.id === representativeId)
          : undefined) ?? profile;
      const modelId = normalizeProviderModelId(provider, representative.modelId);
      const displayName = provider?.display_name ?? representative.displayName;

      if (!isBalanceSupportedProvider(profile.providerId)) {
        balances.push({
          profile_id: representative.id,
          provider_id: profile.providerId,
          model_id: modelId,
          display_name: displayName,
          status: "unsupported",
          error: "暂不支持查询该供应商余额",
        });
        continue;
      }

      const error = fetchErrors.get(profile.providerId);
      if (error) {
        balances.push({
          profile_id: representative.id,
          provider_id: profile.providerId,
          model_id: modelId,
          display_name: displayName,
          status: "error",
          error,
          credential_issue: credentialIssues.get(profile.providerId),
        });
        continue;
      }

      if (!providerKeys.has(profile.providerId)) {
        balances.push({
          profile_id: representative.id,
          provider_id: profile.providerId,
          model_id: modelId,
          display_name: displayName,
          status: "missing_key",
          error: "请先配置 API key",
        });
        continue;
      }

      const detail = fetchedDetails.get(profile.providerId);
      balances.push({
        profile_id: representative.id,
        provider_id: profile.providerId,
        model_id: modelId,
        display_name: displayName,
        status: detail ? "available" : "error",
        fetched_at: new Date().toISOString(),
        detail: detail ?? undefined,
      });
    }

    return balances;
  }

  private async apiKeyForProfile(profileId: string): Promise<string> {
    const store = await this.readStore();
    const profile = store.profiles.find((entry) => entry.id === profileId);
    return profile ? this.readProfileKey(profile) : "";
  }

  private async capabilityResolutionForRequest(
    request: ModelProfileSaveRequest,
    provider: ModelProviderWire,
    baseURL: string,
    selector: string,
  ): Promise<CapabilityResolution | undefined> {
    const profileId = request.id?.trim() || request.templateProfileId?.trim();
    if (!profileId || !selector.trim()) return undefined;
    const store = await this.readStore();
    const stored = store.profiles.find((entry) => entry.id === profileId);
    let credentialRevision = stored?.credentialRevision ?? 1;
    if (provider.id === "volcengine-agent-plan") {
      try {
        credentialRevision = (
          await this.agentPlanAccountStore.getAccount()
        ).credential_revision;
      } catch {
        credentialRevision = 1;
      }
    }
    return capabilityResolutionForIdentity(
      provider,
      profileId,
      credentialRevision,
      baseURL,
      request.protocol?.trim() || stored?.protocol || provider.protocol,
      selector,
    );
  }

  private async readProfileKey(profile: StoredModelProfile): Promise<string> {
    return (await this.readProfileCredential(profile)).apiKey;
  }

  private async readProfileCredential(
    profile: StoredModelProfile,
  ): Promise<{ apiKey: string; credentialRevision?: number }> {
    try {
      if (profile.credentialReference?.kind === "provider_account") {
        const resolved = await this.agentPlanAccountStore.resolveKey();
        this.observedCredentialStates.delete(profile.id);
        return resolved;
      }
      const apiKey = await this.envStore.getKey(profile.id);
      this.observedCredentialStates.delete(profile.id);
      return { apiKey };
    } catch (error) {
      this.observedCredentialStates.set(profile.id, "unreadable");
      throw new AwakenEnvStoreError(
        "unreadable",
        `模型配置「${profile.displayName}」的 API key 无法从 ~/.awaken/.env 读取。`,
        { cause: error },
      );
    }
  }

  private async restoreProfileKey(profileId: string, previousKey: string): Promise<void> {
    if (previousKey) {
      await this.envStore.setKey(profileId, previousKey);
    } else {
      await this.envStore.deleteKey(profileId);
    }
  }

  private async toSummary(profile: StoredModelProfile): Promise<RuntimeModelProfileSummaryWire> {
    let apiKey = "";
    let credentialRevision: number | undefined;
    try {
      const credential = await this.readProfileCredential(profile);
      apiKey = credential.apiKey;
      credentialRevision = credential.credentialRevision;
    } catch {
      // Listing remains available when .env is unreadable so Settings can
      // identify the affected profile and ask the user to re-enter its key.
    }
    const effectiveCredentialRevision =
      credentialRevision ?? profile.credentialRevision ?? 1;
    const provider = providerById(profile.providerId);
    const capabilityModelId = normalizeProviderModelId(provider, profile.modelId);
    const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
    const selector = provider?.endpoint_model
      ? profile.advanced?.endpointId?.trim() || ""
      : capabilityModelId;
    const resolution = provider && selector
      ? capabilityResolutionForIdentity(
          provider,
          profile.id,
          effectiveCredentialRevision,
          baseURL,
          profile.protocol || provider.protocol,
          selector,
        )
      : undefined;
    return toSummary(
      profile,
      apiKey !== "",
      this.observedCredentialStates.get(profile.id),
      effectiveCredentialRevision,
      resolution,
    );
  }

  private ensureSingleDefault(store: StoreFile, preferredProfileId: string): void {
    const defaultProfile =
      store.profiles.find(
        (profile) => profile.id === preferredProfileId && profile.enabled && profile.isDefault,
      ) ?? store.profiles.find((profile) => profile.enabled && profile.isDefault);
    if (!defaultProfile) {
      const firstEnabled = store.profiles.find((profile) => profile.enabled);
      if (firstEnabled) {
        firstEnabled.isDefault = true;
      }
    }
    const defaultID = (
      defaultProfile ?? store.profiles.find((profile) => profile.enabled && profile.isDefault)
    )?.id;
    if (!defaultID) return;
    for (const profile of store.profiles) {
      profile.isDefault = profile.id === defaultID;
    }
  }

  private async readStore(): Promise<StoreFile> {
    try {
      const raw = await fs.readFile(this.filePath, "utf8");
      const parsed = JSON.parse(raw) as Partial<StoreFile>;
      const store: StoreFile = {
        version: parsed.version === 2 ? 2 : 1,
        profiles: Array.isArray(parsed.profiles) ? parsed.profiles : [],
      };
      return this.normalizeStore(store);
    } catch (error) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        return { version: 2, profiles: [] };
      }
      throw error;
    }
  }

  private async writeStore(store: StoreFile): Promise<void> {
    const normalizedStore = this.normalizeStore(store);
    await fs.mkdir(path.dirname(this.filePath), { recursive: true });
    const tempPath = `${this.filePath}.${process.pid}.${randomUUID()}.tmp`;
    try {
      await fs.writeFile(tempPath, `${JSON.stringify(normalizedStore, null, 2)}\n`, "utf8");
      await fs.rename(tempPath, this.filePath);
    } finally {
      await fs.rm(tempPath, { force: true }).catch(() => undefined);
    }
  }

  private async withStoreMutation<T>(operation: () => Promise<T>): Promise<T> {
    const previous = this.mutationQueue;
    let release: (() => void) | undefined;
    this.mutationQueue = new Promise<void>((resolve) => {
      release = resolve;
    });
    await previous;
    try {
      return await operation();
    } finally {
      release?.();
    }
  }

  private normalizeStore(store: StoreFile): StoreFile {
    const normalizedStore: StoreFile = {
      version: 2,
      profiles: dedupeStoredProfiles(store.profiles.map(normalizeStoredProfile)),
    };
    const preferredProfileId =
      normalizedStore.profiles.find((profile) => profile.enabled && profile.isDefault)?.id ??
      normalizedStore.profiles[0]?.id ??
      "";
    this.ensureSingleDefault(normalizedStore, preferredProfileId);
    return normalizedStore;
  }
}
