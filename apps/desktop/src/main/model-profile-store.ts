import { createHash, randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import type {
  AgentPlanAccountSaveRequest,
  AgentPlanAccountWire,
  AgentPlanMediaCapability,
  AgentPlanMediaToolSaveRequest,
  AgentPlanMediaToolsWire,
  ModelReasoningCapabilityTestResult,
  ModelCredentialReference,
  ModelProfileAdvanced,
  ModelProfileBalanceWire,
  ModelUsageMetricWire,
  ModelProfileSaveRequest,
  ModelProfileTestResult,
  ModelProviderWire,
  RuntimeModelProfileSummaryWire,
  ModelProfileRunConfig,
  ModelPurpose,
  ModelPurposeRouting,
  ReasoningEffortCapabilityWire,
  ReasoningEffortLevel,
} from "../shared/runtime.js";
import { VOLCENGINE_AGENT_PLAN_ACCOUNT_ID } from "../shared/runtime.js";
import {
  capabilityForModel,
  effectiveCapabilityForProfile,
  normalizeAdvancedWithModelCapability,
  reasoningEffortCapability,
  resolveReasoningEffort,
  setCapabilityOverrides,
  validateReasoningEffort,
  withModelCapabilities,
} from "./model-capabilities.js";
import {
  capabilityEndpointContract,
  fetchProviderCatalog,
  loadCapabilityOverrides,
  normalizeCapabilityBaseURL,
  parseProviderCapabilities,
  saveCapabilityOverrides,
  type CapabilityResolution,
  type ScopedCapabilityOverrides,
} from "./model-capability-sync.js";
import {
  fetchDeepSeekBalance,
  fetchKimiBalance,
  fetchKimiCodingBalance,
  fetchOpenAIOrganizationUsage,
  fetchAnthropicOrganizationUsage,
  BalanceFetchError,
} from "./model-balance-fetchers.js";
import {
  AwakenEnvStore,
  AwakenEnvStoreError,
} from "./awaken-env-store.js";
import { AgentPlanAccountStore } from "./agent-plan-account-store.js";
import { AgentPlanMediaToolsStore } from "./agent-plan-media-tools-store.js";

const MODEL_PROFILES_FILE = "models.json";
const DUPLICATE_MODEL_PROFILE_ERROR = "模型配置重复，添加失败";
const CAPABILITY_SYNC_TIMEOUT_MS = 8_000;
const BALANCE_FETCH_TIMEOUT_MS = 8_000;
const AGENT_PLAN_EFFORT_PROBE_TIMEOUT_MS = 8_000;
const CONNECTION_TEST_TIMEOUT_MS = 30_000;
const MINIMAX_CN_OPENAI_BASE_URL = "https://api.minimaxi.com/v1";
const MINIMAX_CN_ANTHROPIC_BASE_URL = "https://api.minimaxi.com/anthropic";
const THREE_LLM_PROTOCOLS = [
  "openai_chat",
  "openai_responses",
  "anthropic_messages",
];

export const BUILTIN_MODEL_PROVIDERS: ModelProviderWire[] = [
  {
    id: "openai",
    display_name: "OpenAI",
    base_url: "https://api.openai.com/v1",
    protocol: "openai_responses",
    supported_protocols: ["openai_chat", "openai_responses"],
    official_protocols: ["openai_responses", "openai_chat"],
    catalog_strategy: "openai",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      openai_responses: "bearer",
    },
    key_env_name: "OPENAI_API_KEY",
    default_model: "gpt-5.6-sol",
    models: ["gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"],
    docs_url: "https://platform.openai.com/docs/models",
    api_docs_url: "https://platform.openai.com/api-keys",
    access_type: "api",
    allow_custom_model: true,
  },
  {
    id: "anthropic",
    display_name: "Anthropic",
    base_url: "https://api.anthropic.com",
    protocol: "anthropic_messages",
    supported_protocols: ["anthropic_messages"],
    official_protocols: ["anthropic_messages"],
    protocol_base_urls: {
      anthropic_messages: "https://api.anthropic.com",
    },
    catalog_strategy: "anthropic",
    auth_scheme: "x-api-key",
    protocol_auth_schemes: {
      anthropic_messages: "x-api-key",
    },
    key_env_name: "ANTHROPIC_API_KEY",
    default_model: "claude-fable-5",
    models: ["claude-fable-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-4-5"],
    docs_url: "https://docs.anthropic.com/en/docs/about-claude/models/overview",
    api_docs_url: "https://console.anthropic.com/settings/keys",
    access_type: "api",
    allow_custom_model: true,
  },
  {
    id: "google",
    display_name: "Google Gemini",
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai",
    protocol: "openai_chat",
    supported_protocols: ["openai_chat"],
    official_protocols: ["openai_chat"],
    catalog_strategy: "google",
    auth_scheme: "bearer",
    protocol_auth_schemes: { openai_chat: "bearer" },
    key_env_name: "GEMINI_API_KEY",
    default_model: "gemini-3.6-flash",
    models: ["gemini-3.6-flash", "gemini-3.5-flash-lite"],
    docs_url: "https://ai.google.dev/gemini-api/docs/openai",
    api_docs_url: "https://aistudio.google.com/apikey",
    access_type: "api",
    allow_custom_model: true,
  },
  {
    id: "custom",
    display_name: "自定义 API",
    base_url: "",
    protocol: "openai_chat",
    supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
    // A custom endpoint has no vendor-owned contract. The user explicitly
    // declares which of Awaken's three adapters the gateway implements.
    official_protocols: [],
    catalog_strategy: "none",
    auth_scheme: "bearer",
    key_env_name: "CUSTOM_LLM_API_KEY",
    default_model: "",
    models: [],
    docs_url: "",
    api_docs_url: "",
    access_type: "custom",
    allow_custom_model: true,
  },
  {
    id: "kimi-coding-plan",
    display_name: "Kimi Coding Plan",
    base_url: "https://api.kimi.com/coding/v1",
    protocol: "openai_chat",
    supported_protocols: ["openai_chat", "anthropic_messages"],
    official_protocols: ["openai_chat", "anthropic_messages"],
    protocol_base_urls: {
      openai_chat: "https://api.kimi.com/coding/v1",
      anthropic_messages: "https://api.kimi.com/coding",
    },
    catalog_strategy: "none",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      anthropic_messages: "bearer",
    },
    key_env_name: "KIMI_CODE_API_KEY",
    default_model: "k3",
    models: ["k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"],
    docs_url: "https://www.kimi.com/code/docs/kimi-code/models.html",
    api_docs_url: "https://www.kimi.com/code/console",
    access_type: "coding_plan",
    coding_preset: true,
  },
  {
    id: "minimax-token-plan",
    display_name: "MiniMax Token Plan",
    base_url: MINIMAX_CN_OPENAI_BASE_URL,
    protocol: "openai_chat",
    supported_protocols: [...THREE_LLM_PROTOCOLS],
    official_protocols: [...THREE_LLM_PROTOCOLS],
    protocol_base_urls: {
      openai_chat: MINIMAX_CN_OPENAI_BASE_URL,
      openai_responses: MINIMAX_CN_OPENAI_BASE_URL,
      anthropic_messages: MINIMAX_CN_ANTHROPIC_BASE_URL,
    },
    catalog_strategy: "openai",
    catalog_url: `${MINIMAX_CN_OPENAI_BASE_URL}/models`,
    catalog_auth_scheme: "bearer",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      openai_responses: "bearer",
      anthropic_messages: "x-api-key",
    },
    key_env_name: "MINIMAX_TOKEN_PLAN_API_KEY",
    default_model: "MiniMax-M3",
    models: ["MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.5"],
    docs_url: "https://platform.minimaxi.com/docs/token-plan/quickstart",
    api_docs_url: "https://platform.minimaxi.com/subscribe/token-plan",
    access_type: "coding_plan",
    coding_preset: true,
    allow_custom_model: true,
  },
  {
    id: "glm-coding-plan",
    display_name: "GLM Coding Plan",
    base_url: "https://open.bigmodel.cn/api/coding/paas/v4",
    protocol: "openai_chat",
    supported_protocols: ["openai_chat", "anthropic_messages"],
    official_protocols: ["openai_chat", "anthropic_messages"],
    protocol_base_urls: {
      openai_chat: "https://open.bigmodel.cn/api/coding/paas/v4",
      anthropic_messages: "https://open.bigmodel.cn/api/anthropic",
    },
    catalog_strategy: "none",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      anthropic_messages: "x-api-key",
    },
    key_env_name: "GLM_CODING_PLAN_API_KEY",
    default_model: "glm-5.2",
    models: ["glm-5.2", "glm-5-turbo", "glm-4.7"],
    docs_url: "https://docs.bigmodel.cn/cn/coding-plan/quick-start",
    api_docs_url: "https://bigmodel.cn/usercenter/proj-mgmt/apikeys",
    access_type: "coding_plan",
    coding_preset: true,
    allow_custom_model: true,
  },
  {
    id: "volcengine",
    display_name: "火山 Coding Plan",
    base_url: "https://ark.cn-beijing.volces.com/api/coding/v3",
    protocol: "openai_chat",
    supported_protocols: ["openai_chat", "anthropic_messages"],
    official_protocols: ["openai_chat", "anthropic_messages"],
    protocol_base_urls: {
      openai_chat: "https://ark.cn-beijing.volces.com/api/coding/v3",
      anthropic_messages: "https://ark.cn-beijing.volces.com/api/coding",
    },
    catalog_strategy: "openai",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      anthropic_messages: "bearer",
    },
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
    supported_protocols: [...THREE_LLM_PROTOCOLS],
    official_protocols: [...THREE_LLM_PROTOCOLS],
    protocol_base_urls: {
      openai_chat: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
      openai_responses: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1",
      anthropic_messages: "https://token-plan.cn-beijing.maas.aliyuncs.com/apps/anthropic",
    },
    catalog_strategy: "openai",
    catalog_url: "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/models",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      openai_responses: "bearer",
      anthropic_messages: "x-api-key",
    },
    key_env_name: "DASHSCOPE_TOKEN_PLAN_API_KEY",
    default_model: "qwen3.8-max-preview",
    models: ["qwen3.8-max-preview"],
    docs_url: "https://help.aliyun.com/zh/model-studio/token-plan",
    api_docs_url: "https://bailian.console.aliyun.com/",
    access_type: "coding_plan",
    coding_preset: true,
    allow_custom_model: true,
  },
  {
    id: "kimi-cn",
    display_name: "Kimi",
    base_url: "https://api.moonshot.cn/v1",
    protocol: "openai_chat",
    supported_protocols: ["openai_chat"],
    official_protocols: ["openai_chat"],
    catalog_strategy: "openai",
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
    base_url: MINIMAX_CN_OPENAI_BASE_URL,
    protocol: "openai_chat",
    supported_protocols: [...THREE_LLM_PROTOCOLS],
    official_protocols: [...THREE_LLM_PROTOCOLS],
    protocol_base_urls: {
      openai_chat: MINIMAX_CN_OPENAI_BASE_URL,
      openai_responses: MINIMAX_CN_OPENAI_BASE_URL,
      anthropic_messages: MINIMAX_CN_ANTHROPIC_BASE_URL,
    },
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      openai_responses: "bearer",
      anthropic_messages: "x-api-key",
    },
    key_env_name: "MINIMAX_API_KEY",
    default_model: "MiniMax-M3",
    models: ["MiniMax-M3"],
    docs_url: "https://platform.minimaxi.com/docs/api-reference/api-overview",
    api_docs_url: "https://platform.minimaxi.com/console/access?tab=api-keys",
    access_type: "api",
    catalog_strategy: "openai",
    catalog_url: `${MINIMAX_CN_OPENAI_BASE_URL}/models`,
    catalog_auth_scheme: "bearer",
  },
  {
    id: "glm",
    display_name: "GLM",
    base_url: "https://open.bigmodel.cn/api/paas/v4",
    protocol: "openai_chat",
    supported_protocols: ["openai_chat", "anthropic_messages"],
    official_protocols: ["openai_chat", "anthropic_messages"],
    protocol_base_urls: {
      openai_chat: "https://open.bigmodel.cn/api/paas/v4",
      anthropic_messages: "https://open.bigmodel.cn/api/anthropic",
    },
    catalog_strategy: "openai",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      anthropic_messages: "x-api-key",
    },
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
    supported_protocols: [...THREE_LLM_PROTOCOLS],
    official_protocols: [...THREE_LLM_PROTOCOLS],
    protocol_base_urls: {
      openai_chat: "https://dashscope.aliyuncs.com/compatible-mode/v1",
      openai_responses: "https://dashscope.aliyuncs.com/compatible-mode/v1",
      anthropic_messages: "https://dashscope.aliyuncs.com/apps/anthropic",
    },
    catalog_strategy: "openai",
    catalog_url: "https://dashscope.aliyuncs.com/api/v1/models",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      openai_responses: "bearer",
      anthropic_messages: "x-api-key",
    },
    key_env_name: "DASHSCOPE_API_KEY",
    default_model: "qwen3.8-max",
    models: ["qwen3.8-max"],
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
    supported_protocols: ["openai_chat", "openai_responses"],
    official_protocols: ["openai_chat", "openai_responses"],
    protocol_base_urls: {
      openai_chat: "https://ark.cn-beijing.volces.com/api/v3",
      openai_responses: "https://ark.cn-beijing.volces.com/api/v3",
    },
    catalog_strategy: "none",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      openai_responses: "bearer",
    },
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
    supported_protocols: [...THREE_LLM_PROTOCOLS],
    official_protocols: [...THREE_LLM_PROTOCOLS],
    protocol_base_urls: {
      openai_chat: "https://api.deepseek.com",
      openai_responses: "https://api.deepseek.com",
      anthropic_messages: "https://api.deepseek.com/anthropic",
    },
    catalog_strategy: "openai",
    catalog_url: "https://api.deepseek.com/models",
    catalog_auth_scheme: "bearer",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      openai_responses: "bearer",
      anthropic_messages: "x-api-key",
    },
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
    supported_protocols: ["openai_chat", "anthropic_messages"],
    official_protocols: ["openai_chat", "anthropic_messages"],
    protocol_base_urls: {
      openai_chat: "https://ark.cn-beijing.volces.com/api/plan/v3",
      anthropic_messages: "https://ark.cn-beijing.volces.com/api/plan",
    },
    catalog_strategy: "openai",
    auth_scheme: "bearer",
    protocol_auth_schemes: {
      openai_chat: "bearer",
      anthropic_messages: "bearer",
    },
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
  version: 1 | 2 | 3;
  profiles: StoredModelProfile[];
  purposeRoutes?: ModelPurposeRouting;
};

export interface ModelProfileKeyStore {
  getKey(profileId: string): Promise<string>;
  setKey(profileId: string, value: string): Promise<void>;
  deleteKey(profileId: string): Promise<void>;
  getManagementKey?(providerId: string): Promise<string>;
  setManagementKey?(providerId: string, value: string): Promise<void>;
  deleteManagementKey?(providerId: string): Promise<void>;
}

function providerById(providerId: string): ModelProviderWire | undefined {
  return BUILTIN_MODEL_PROVIDERS.find((provider) => provider.id === providerId);
}

function trimTrailingSlash(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

const SUPPORTED_PROTOCOLS = new Set([
  "openai_chat",
  "openai_responses",
  "anthropic_messages",
  "minimax_chat",
]);
const SUPPORTED_AUTH_SCHEMES = new Set([
  "bearer",
  "x-api-key",
  "api-key",
  "x-goog-api-key",
  "none",
]);

function normalizeProtocol(provider: ModelProviderWire | undefined, value: string | undefined): string {
  const protocol = value?.trim().toLowerCase() || provider?.protocol || "";
  if (!SUPPORTED_PROTOCOLS.has(protocol)) throw new Error(`不支持的模型协议：${protocol || "空"}`);
  const allowed = provider?.supported_protocols?.length
    ? provider.supported_protocols
    : provider
      ? [provider.protocol]
      : [];
  if (provider?.id === "minimax" && protocol === "minimax_chat") {
    return protocol;
  }
  if (allowed.length > 0 && !allowed.includes(protocol)) {
    throw new Error(`${provider?.display_name ?? "该供应商"} 不支持协议 ${protocol}`);
  }
  return protocol;
}

function normalizeAuthScheme(provider: ModelProviderWire | undefined, value: string | undefined): string {
  const authScheme = value?.trim().toLowerCase() || provider?.auth_scheme || "bearer";
  if (!SUPPORTED_AUTH_SCHEMES.has(authScheme)) throw new Error(`不支持的鉴权方式：${authScheme}`);
  return authScheme;
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
  if (provider?.id === "minimax" || provider?.id === "minimax-token-plan") {
    if (normalized === "https://api.minimax.io/v1") {
      return MINIMAX_CN_OPENAI_BASE_URL;
    }
    if (normalized === "https://api.minimax.io/anthropic") {
      return MINIMAX_CN_ANTHROPIC_BASE_URL;
    }
  }
  if (provider?.id === "volcengine-agent-plan") {
    const supportedBases = new Set([
      trimTrailingSlash(provider.base_url),
      ...Object.values(provider.protocol_base_urls ?? {}).map(trimTrailingSlash),
    ]);
    return supportedBases.has(normalized) ? normalized : provider.base_url;
  }
  return normalized;
}

function validateProtocolEndpointContract(
  provider: ModelProviderWire,
  protocol: string,
  baseURL: string,
  inferenceEndpoint?: string,
): void {
  const officialProtocols = provider.official_protocols?.length
    ? provider.official_protocols
    : [provider.protocol];
  if (provider.id === "minimax" && protocol === "minimax_chat") return;
  if (provider.id === "custom") return;
  const protocolBaseURL = provider.protocol_base_urls?.[protocol];
  if (officialProtocols.includes(protocol)) {
    const usesProviderDefault =
      normalizeCapabilityBaseURL(baseURL) ===
      normalizeCapabilityBaseURL(provider.base_url);
    if (
      protocolBaseURL &&
      usesProviderDefault &&
      normalizeCapabilityBaseURL(protocolBaseURL) !==
        normalizeCapabilityBaseURL(provider.base_url) &&
      !inferenceEndpoint?.trim()
    ) {
      throw new Error(
        `${provider.display_name} 的 ${protocol} 官方 Base URL 是 ${protocolBaseURL}，请使用该地址或填写完整推理地址。`,
      );
    }
    return;
  }
  const usesOfficialBase =
    normalizeCapabilityBaseURL(baseURL) ===
    normalizeCapabilityBaseURL(provider.base_url);
  if (!usesOfficialBase || inferenceEndpoint?.trim()) return;
  throw new Error(
    `${provider.display_name} 的官方地址未确认支持 ${protocol}；请填写兼容 Base URL 或完整推理地址。`,
  );
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

const BALANCE_SUPPORTED_PROVIDERS = new Set<string>([
  "deepseek",
  "kimi-cn",
  "kimi-coding-plan",
  "openai",
  "anthropic",
  "google",
]);

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
    case "kimi-cn":
      return await fetchKimiBalance(baseURL, apiKey, signal);
    case "kimi-coding-plan":
      return await fetchKimiCodingBalance(baseURL, apiKey, signal);
    case "openai":
      return await fetchOpenAIOrganizationUsage(apiKey, signal);
    case "anthropic":
      return await fetchAnthropicOrganizationUsage(apiKey, signal);
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
  if (profile.authScheme === "none") return "ready";
  if (!hasKey) return "missing_key";
  return "ready";
}

function providerErrorDetail(body: unknown): string | undefined {
  if (typeof body !== "object" || body === null) return undefined;
  const root = body as Record<string, unknown>;
  const error = typeof root.error === "object" && root.error !== null
    ? root.error as Record<string, unknown>
    : root;
  const code = typeof error.code === "string" || typeof error.code === "number"
    ? String(error.code)
    : typeof root.code === "string" || typeof root.code === "number"
      ? String(root.code)
      : "";
  const rawMessage = typeof error.message === "string"
    ? error.message
    : typeof root.message === "string"
      ? root.message
      : "";
  const message = rawMessage
    .replace(/authorization\s*:\s*bearer\s+\S+/gi, "Authorization: Bearer [REDACTED]")
    .replace(/\b(sk|ak)-[A-Za-z0-9_-]{8,}\b/g, "$1-[REDACTED]")
    .replace(/[\r\n\t]+/g, " ")
    .trim()
    .slice(0, 240);
  const parts = [code, message].filter(Boolean);
  return parts.length > 0 ? parts.join(": ") : undefined;
}

function providerTestStatusMessage(status: number, provider?: ModelProviderWire, body?: unknown): string {
  const detail = providerErrorDetail(body);
  const withDetail = (message: string) => detail ? `${message}（供应商：${detail}）` : message;
  if (provider?.id === "kimi-coding-plan") {
    switch (status) {
      case 401:
        return withDetail("连接测试失败：API key 无效，或当前会员套餐无权访问所选 Kimi Code 模型（K3 需 Moderato 及以上，高速版需 Allegretto 及以上）。");
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
      return withDetail("连接测试失败：API key 无效或未授权。");
    case 403:
      return withDetail("连接测试失败：API key 无权访问该服务。");
    case 404:
      return withDetail("连接测试失败：推理端点或模型不存在，请检查协议、模型 ID 与服务地址。");
    case 429:
      return withDetail("连接测试失败：请求被限流，请稍后重试。");
    default:
      if (status >= 500) {
        return withDetail(`连接测试失败：服务端返回 HTTP ${status}。`);
      }
      return withDetail(`连接测试失败：服务返回 HTTP ${status}。`);
  }
}

async function testModelEndpoint(
  provider: ModelProviderWire,
  baseURL: string,
  inferenceEndpoint: string | undefined,
  protocol: string,
  authScheme: string,
  modelId: string,
  apiKey: string,
  signal: AbortSignal,
): Promise<Response> {
  const body = connectionTestBody(provider, protocol, modelId);
  const normalizedBaseURL = trimTrailingSlash(baseURL);
  const endpoint = inferenceEndpoint?.trim() || (() => {
    switch (protocol) {
      case "minimax_chat":
        return `${normalizedBaseURL}/text/chatcompletion_v2`;
      case "openai_responses":
        return `${normalizedBaseURL}/responses`;
      case "anthropic_messages":
        return /\/v1$/i.test(normalizedBaseURL)
          ? `${normalizedBaseURL}/messages`
          : `${normalizedBaseURL}/v1/messages`;
      case "openai_chat":
        return `${normalizedBaseURL}/chat/completions`;
      default:
        throw new Error(`不支持的模型协议：${protocol}`);
    }
  })();
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  switch (authScheme) {
    case "x-api-key": headers["x-api-key"] = apiKey; break;
    case "api-key": headers["api-key"] = apiKey; break;
    case "x-goog-api-key": headers["x-goog-api-key"] = apiKey; break;
    case "none": break;
    default: headers.Authorization = `Bearer ${apiKey}`; break;
  }
  if (protocol === "anthropic_messages") headers["anthropic-version"] = "2023-06-01";
  return fetch(endpoint, {
    method: "POST",
    headers,
    body,
    signal,
  });
}

function connectionTestBody(provider: ModelProviderWire, protocol: string, modelId: string): string {
  const model = modelId || provider.default_model;
  switch (protocol) {
    case "openai_responses":
      return JSON.stringify({ model, input: "ping", stream: false, store: false });
    case "anthropic_messages":
      return JSON.stringify({ model, messages: [{ role: "user", content: "ping" }], max_tokens: 64, stream: false });
    default:
      return JSON.stringify({
        model,
        messages: [{ role: "user", content: "ping" }],
        ...(provider.id === "kimi-coding-plan" ? { temperature: 1 } : {}),
        stream: false,
      });
  }
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

function providerTestSuccessMessage(provider: ModelProviderWire, protocol: string): string {
  if (provider.id === "kimi-coding-plan") {
    return "连接测试通过：Kimi Code 已响应 chat/completions。";
  }
  if (provider.id === "volcengine") {
    return "连接测试通过：火山 Coding Plan 已响应 chat/completions。";
  }
  const endpoint = protocol === "openai_responses"
    ? "responses"
    : protocol === "anthropic_messages"
      ? "messages"
      : protocol === "minimax_chat"
        ? "chatcompletion_v2"
        : "chat/completions";
  return `连接测试通过：服务已响应 ${endpoint}。`;
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

function rateLimitObservation(
  headers: Pick<Headers, "get"> | undefined,
): Record<string, unknown> | undefined {
  const candidates = {
    request_limit: headers?.get("x-ratelimit-limit-requests") ?? null,
    request_remaining: headers?.get("x-ratelimit-remaining-requests") ?? null,
    request_reset: headers?.get("x-ratelimit-reset-requests") ?? null,
    token_limit: headers?.get("x-ratelimit-limit-tokens") ?? null,
    token_remaining: headers?.get("x-ratelimit-remaining-tokens") ?? null,
    token_reset: headers?.get("x-ratelimit-reset-tokens") ?? null,
    retry_after: headers?.get("retry-after") ?? null,
  };
  const values = Object.fromEntries(Object.entries(candidates).filter(([, value]) => value !== null));
  return Object.keys(values).length > 0
    ? { observed_at: new Date().toISOString(), ...values }
    : undefined;
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
  managementKey?: string,
): RuntimeModelProfileSummaryWire {
  const provider = providerById(profile.providerId);
  const modelId = normalizeProviderModelId(provider, profile.modelId);
  const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
  const advanced = normalizeAdvancedWithModelCapability(
    profile.advanced,
    profile.providerId,
    modelId,
    resolution,
    profile.protocol,
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
    has_management_key: Boolean(managementKey),
    masked_management_key: managementKey ? maskKey(managementKey.slice(-4)) : undefined,
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
      inference_endpoint: advanced.inferenceEndpoint,
      max_input_tokens: advanced.maxInputTokens,
      context_window: advanced.contextWindow,
      supports_vision: advanced.supportsVision,
      supports_native_pdf: advanced.supportsNativePDF,
      supports_video: advanced.supportsVideo,
      supports_tools: advanced.supportsTools,
      supports_prompt_cache: advanced.supportsPromptCache,
      default_reasoning_effort: advanced.defaultReasoningEffort,
    },
    credential_revision: credentialRevision ?? profile.credentialRevision ?? 1,
    effective_capability: effectiveCapabilityForProfile(
      advanced,
      profile.providerId,
      modelId,
      resolution,
      profile.protocol,
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
  const selector = provider?.endpoint_model
    ? profile.advanced?.endpointId?.trim() || modelId
    : modelId;
  const baseURL = normalizeCapabilityBaseURL(
    normalizeProviderBaseURL(provider, profile.baseURL),
  );
  const protocol = normalizeProtocol(provider, profile.protocol);
  return `${profile.providerId}::${baseURL}::${selector}::${protocol}`;
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
  const provider = providerById(profile.providerId);
  const keyLast4 = typeof profile.keyLast4 === "string" ? profile.keyLast4.slice(-4) : undefined;
  const storedProtocol = profile.protocol?.trim().toLowerCase();
  const providerProtocols = provider?.supported_protocols?.length
    ? provider.supported_protocols
    : provider
      ? [provider.protocol]
      : [];
  const protocolWasRetired = Boolean(
    provider &&
      storedProtocol &&
      SUPPORTED_PROTOCOLS.has(storedProtocol) &&
      !providerProtocols.includes(storedProtocol),
  );
  const protocol = protocolWasRetired
    ? provider!.protocol
    : normalizeProtocol(provider, storedProtocol);
  const knownProviderBaseURLs = new Set(
    provider
      ? [
          provider.base_url,
          ...Object.values(provider.protocol_base_urls ?? {}),
        ].map((value) => normalizeCapabilityBaseURL(value))
      : [],
  );
  const baseURL =
    protocolWasRetired &&
    provider &&
    knownProviderBaseURLs.has(normalizeCapabilityBaseURL(profile.baseURL))
      ? provider.protocol_base_urls?.[protocol] ?? provider.base_url
      : normalizeProviderBaseURL(provider, profile.baseURL);
  const knownProviderAuthSchemes = new Set(
    provider
      ? [
          provider.auth_scheme,
          ...Object.values(provider.protocol_auth_schemes ?? {}),
        ]
      : [],
  );
  const authScheme =
    protocolWasRetired &&
    provider &&
    knownProviderAuthSchemes.has(profile.authScheme)
      ? provider.protocol_auth_schemes?.[protocol] ?? provider.auth_scheme
      : normalizeAuthScheme(provider, profile.authScheme);
  return {
    id: profile.id,
    providerId: profile.providerId,
    displayName: profile.displayName,
    modelId: profile.modelId,
    baseURL,
    protocol,
    authScheme,
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
  private agentPlanMediaToolsStore: AgentPlanMediaToolsStore;
  private agentPlanModelIds = ["ark-code-latest"];
  private observedCredentialStates = new Map<string, "unreadable">();
  private mutationQueue: Promise<void> = Promise.resolve();
  private capabilityOverrides: ScopedCapabilityOverrides[] = [];

  constructor(
    awakenHome: string,
    envStore: ModelProfileKeyStore = new AwakenEnvStore(awakenHome),
    agentPlanAccountStore: AgentPlanAccountStore = new AgentPlanAccountStore(
      awakenHome,
    ),
    agentPlanMediaToolsStore: AgentPlanMediaToolsStore = new AgentPlanMediaToolsStore(
      awakenHome,
      agentPlanAccountStore,
    ),
  ) {
    this.filePath = path.join(awakenHome, "config", MODEL_PROFILES_FILE);
    this.cacheDir = path.join(awakenHome, "cache");
    this.usageDir = path.join(awakenHome, "usage", "model-profiles");
    this.envStore = envStore;
    this.agentPlanAccountStore = agentPlanAccountStore;
    this.agentPlanMediaToolsStore = agentPlanMediaToolsStore;
  }

  async initialize(): Promise<void> {
    this.capabilityOverrides = await loadCapabilityOverrides(this.cacheDir);
    setCapabilityOverrides(this.capabilityOverrides);
    try {
      const account = await this.agentPlanAccountStore.getAccount();
      this.agentPlanModelIds = account.catalog
        .filter((entry) => entry.kind === "llm")
        .map((entry) => entry.id);
    } catch {
      this.agentPlanModelIds = ["ark-code-latest"];
    }
  }

  async syncCapabilities(): Promise<ScopedCapabilityOverrides[]> {
    const store = await this.readStore();
    const candidates: Array<{
      baseURL: string;
      apiKey: string;
      provider: ModelProviderWire;
      authScheme: string;
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
      if (!credential.apiKey && profile.authScheme !== "none") continue;
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
        provider,
        authScheme: profile.authScheme,
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
      candidates.map(async ({ baseURL, apiKey, provider, authScheme, resolution }) => {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), CAPABILITY_SYNC_TIMEOUT_MS);
        try {
          const catalog = await fetchProviderCatalog(
            provider.catalog_strategy ?? "openai",
            baseURL,
            apiKey,
            provider.catalog_auth_scheme ?? authScheme,
            controller.signal,
            provider.catalog_url,
          );
          fetched.push({
            scope: resolution.scope,
            models: catalog.capabilities,
            catalog_models: catalog.models,
            catalog_entries: catalog.entries,
            observed_at: catalog.observedAt,
          });
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

  async refreshProviderCatalog(
    request: ModelProfileSaveRequest,
  ): Promise<{ provider: ModelProviderWire; message: string }> {
    const provider = providerById(request.providerId);
    if (!provider) throw new Error("未知模型供应商");
    const strategy = provider.catalog_strategy ?? "openai";
    const builtin = withModelCapabilities({ ...provider });
    if (strategy === "none") {
      return {
        provider: {
          ...builtin,
          catalog_source: "unavailable",
        },
        message: `${provider.display_name} 未提供可由当前推理凭据调用的模型目录 API，请使用官方清单或手填模型 ID。`,
      };
    }

    const store = await this.readStore();
    const storedProfile = request.id
      ? store.profiles.find((profile) => profile.id === request.id)
      : request.templateProfileId
        ? store.profiles.find((profile) => profile.id === request.templateProfileId)
        : undefined;
    let apiKey = request.apiKey?.trim() ?? "";
    let credentialRevision = storedProfile?.credentialRevision ?? 1;
    if (!apiKey && provider.id === "volcengine-agent-plan") {
      const account = await this.agentPlanAccountStore.getAccount();
      const credential = await this.agentPlanAccountStore.resolveKey();
      apiKey = credential.apiKey;
      credentialRevision = account.credential_revision;
    } else if (!apiKey && storedProfile) {
      const credential = await this.readProfileCredential(storedProfile);
      apiKey = credential.apiKey;
      credentialRevision = credential.credentialRevision ?? credentialRevision;
    }

    const authScheme = normalizeAuthScheme(provider, request.authScheme);
    if (!apiKey && authScheme !== "none") {
      throw new Error("请先填写 API key，再刷新模型目录。");
    }
    const baseURL = normalizeProviderBaseURL(provider, request.baseURL);
    if (!baseURL) throw new Error("请先填写 Base URL，再刷新模型目录。");

    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), CAPABILITY_SYNC_TIMEOUT_MS);
    try {
      const catalog = await fetchProviderCatalog(
        strategy,
        baseURL,
        apiKey,
        provider.catalog_auth_scheme ?? authScheme,
        controller.signal,
        provider.catalog_url,
      );
      const protocol = normalizeProtocol(provider, request.protocol);
      const selector = provider.endpoint_model
        ? request.advanced?.endpointId?.trim() || ""
        : normalizeProviderModelId(provider, request.modelId);
      if (storedProfile && selector) {
        const resolution = capabilityResolutionForIdentity(
          provider,
          storedProfile.id,
          credentialRevision,
          baseURL,
          protocol,
          selector,
        );
        this.capabilityOverrides = await saveCapabilityOverrides(this.cacheDir, [{
          scope: resolution.scope,
          models: catalog.capabilities,
          catalog_models: catalog.models,
          catalog_entries: catalog.entries,
          observed_at: catalog.observedAt,
        }]);
        setCapabilityOverrides(this.capabilityOverrides);
      }
      return {
        provider: {
          ...builtin,
          models: Array.from(new Set([...provider.models, ...catalog.models])).sort(),
          model_capabilities: {
            ...(builtin.model_capabilities ?? {}),
            ...catalog.capabilities,
          },
          catalog_source: "provider_api",
          catalog_observed_at: catalog.observedAt,
        },
        message: catalog.models.length > 0
          ? `已从 ${provider.display_name} API 同步 ${catalog.models.length} 个模型。`
          : `${provider.display_name} 模型目录 API 已响应，但没有返回可用模型。`,
      };
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        throw new Error("模型目录同步超时，请检查网络后重试。");
      }
      throw error;
    } finally {
      clearTimeout(timeout);
    }
  }

  listProviders(): ModelProviderWire[] {
    return BUILTIN_MODEL_PROVIDERS.map((provider) => {
      const cachedCatalogs = this.capabilityOverrides.filter(
        (entry) => entry.scope.provider_id === provider.id,
      );
      const latestObservedAt = cachedCatalogs
        .map((entry) => entry.observed_at)
        .filter((value): value is string => Boolean(value))
        .sort()
        .at(-1);
      return withModelCapabilities({
        ...provider,
        models:
          provider.id === "volcengine-agent-plan"
            ? [...provider.models]
            : Array.from(new Set([
                ...provider.models,
                ...cachedCatalogs.flatMap((entry) => entry.catalog_models ?? []),
              ])).sort(),
        catalog_source:
          cachedCatalogs.length > 0 ? "provider_api" : "official_manifest",
        catalog_observed_at: latestObservedAt,
        allow_custom_model:
          provider.id === "volcengine-agent-plan"
            ? false
            : provider.allow_custom_model,
      });
    });
  }

  async getAgentPlanAccount(): Promise<AgentPlanAccountWire> {
    return this.agentPlanAccountStore.getAccount();
  }

  async saveAgentPlanAccount(request: AgentPlanAccountSaveRequest): Promise<AgentPlanAccountWire> {
    const account = await this.agentPlanAccountStore.saveAccount(request);
    this.agentPlanModelIds = account.catalog
      .filter((entry) => entry.kind === "llm")
      .map((entry) => entry.id);
    return account;
  }

  async refreshAgentPlanModels(): Promise<AgentPlanAccountWire> {
    const account = await this.agentPlanAccountStore.refreshModelCatalog();
    this.agentPlanModelIds = account.catalog
      .filter((entry) => entry.kind === "llm")
      .map((entry) => entry.id);
    return account;
  }

  async getAgentPlanMediaTools(): Promise<AgentPlanMediaToolsWire> {
    return this.agentPlanMediaToolsStore.snapshot();
  }

  async saveAgentPlanMediaTool(
    request: AgentPlanMediaToolSaveRequest,
  ): Promise<AgentPlanMediaToolsWire> {
    return this.agentPlanMediaToolsStore.save(request);
  }

  async deleteAgentPlanMediaTool(
    capability: AgentPlanMediaCapability,
  ): Promise<AgentPlanMediaToolsWire> {
    return this.agentPlanMediaToolsStore.delete(capability);
  }

  async deleteAgentPlanAccount(): Promise<void> {
    const store = await this.readStore();
    const profileReferenced = store.profiles.some(
      (profile) => profile.credentialReference?.kind === "provider_account",
    );
    const toolReferenced =
      await this.agentPlanMediaToolsStore.hasAccountReferences();
    await this.agentPlanAccountStore.deleteAccount(
      profileReferenced || toolReferenced,
    );
    this.agentPlanModelIds = ["ark-code-latest"];
  }

  async listProfiles(): Promise<RuntimeModelProfileSummaryWire[]> {
    const store = await this.readStore();
    return Promise.all(
      store.profiles
        .filter(
          (profile) =>
            profile.providerId !== "volcengine-agent-plan" ||
            this.agentPlanModelIds.includes(profile.modelId.trim()),
        )
        .map((profile) => this.toSummary(profile)),
    );
  }

  async getPurposeRouting(): Promise<ModelPurposeRouting> {
    const store = await this.readStore();
    return { ...(store.purposeRoutes ?? {}) };
  }

  async savePurposeRouting(routing: ModelPurposeRouting): Promise<ModelPurposeRouting> {
    return this.withStoreMutation(async () => {
      const store = await this.readStore();
      const validIDs = new Set(store.profiles.map((profile) => profile.id));
      const next: ModelPurposeRouting = {};
      for (const purpose of ["thread_title", "compact_summary", "subagent", "scheduled_task"] as ModelPurpose[]) {
        const profileID = routing[purpose]?.trim();
        if (profileID && validIDs.has(profileID)) next[purpose] = profileID;
      }
      store.purposeRoutes = next;
      await this.writeStore(store);
      return { ...next };
    });
  }

  async resolvePurposeRunProfile(
    purpose: ModelPurpose,
    inheritedProfileId?: string,
  ): Promise<ModelProfileRunConfig | undefined> {
    const store = await this.readStore();
    const routedID = store.purposeRoutes?.[purpose];
    const routedProfile = routedID
      ? store.profiles.find((profile) => profile.id === routedID && profile.enabled)
      : undefined;
    if (routedProfile) {
      try {
        return await this.resolveRunProfile(routedProfile.id);
      } catch {
        // A stale optional route inherits the caller's profile. Explicit
        // scheduled-task pins are resolved by Core and still fail closed.
      }
    }
    return this.resolveRunProfile(inheritedProfileId);
  }

  async saveProfile(request: ModelProfileSaveRequest): Promise<RuntimeModelProfileSummaryWire> {
    const provider = providerById(request.providerId);
    if (!provider) {
      throw new Error("未知模型供应商");
    }
    if (
      provider.id === "volcengine-agent-plan" &&
      !this.agentPlanModelIds.includes(request.modelId.trim())
    ) {
      throw new Error(
        "Agent Plan 主模型只能选择目录中已分类的对话或推理模型。",
      );
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
    const protocol = normalizeProtocol(provider, request.protocol);
    const authScheme = normalizeAuthScheme(provider, request.authScheme);
    if (!baseURL) throw new Error("请填写 Base URL。");
    if (!modelId && !provider.endpoint_model) throw new Error("请填写模型 ID。");
    if (request.advanced?.inferenceEndpoint?.trim()) {
      try {
        const endpoint = new URL(request.advanced.inferenceEndpoint.trim());
        if (endpoint.protocol !== "https:" && endpoint.protocol !== "http:") throw new Error();
      } catch {
        throw new Error("完整推理地址必须是有效的 HTTP(S) URL。");
      }
    }
    validateProtocolEndpointContract(
      provider,
      protocol,
      baseURL,
      request.advanced?.inferenceEndpoint,
    );
    const existing = existingIndex >= 0 ? store.profiles[existingIndex] : undefined;
    const templateProfile = templateProfileForSave(
      store,
      provider.id,
      request.templateProfileId,
    );
    const candidateIdentity = storedProfileIdentityKey({
      id: existing?.id ?? "candidate",
      providerId: provider.id,
      displayName: request.displayName?.trim() || provider.display_name,
      modelId,
      baseURL,
      protocol,
      authScheme,
      enabled: true,
      isDefault: false,
      advanced: request.advanced ?? existing?.advanced ?? templateProfile?.advanced,
    });
    const duplicateProfile = store.profiles.find(
      (profile) => profile.id !== existing?.id && storedProfileIdentityKey(profile) === candidateIdentity,
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
      protocol,
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
      protocol,
      authScheme,
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
    const managementKeyToPersist =
      (provider.id === "openai" || provider.id === "anthropic") && request.managementApiKey?.trim()
        ? request.managementApiKey.trim()
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
    const previousManagementKey = managementKeyToPersist && this.envStore.getManagementKey
      ? await this.envStore.getManagementKey(provider.id)
      : undefined;
    if (managementKeyToPersist && this.envStore.setManagementKey) {
      await this.envStore.setManagementKey(provider.id, managementKeyToPersist);
    }
    try {
      if (agentPlanKeyToPersist !== undefined) {
        const account =
          await this.agentPlanAccountStore.saveAccountWithCompensation(
            { apiKey: agentPlanKeyToPersist },
            () => this.writeStore(store),
          );
        this.agentPlanModelIds = account.catalog
          .filter((entry) => entry.kind === "llm")
          .map((entry) => entry.id);
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
    } catch (error) {
      if (managementKeyToPersist) {
        await this.restoreManagementKey(provider.id, previousManagementKey ?? "");
      }
      throw error;
    }
    return this.toSummary(profile);
  }

  async deleteProfile(profileId: string): Promise<void> {
    await this.withStoreMutation(async () => {
      const store = await this.readStore();
      const previousDefault = store.profiles.find((profile) => profile.id === profileId)?.isDefault;
      store.profiles = store.profiles.filter((profile) => profile.id !== profileId);
      if (store.purposeRoutes) {
        for (const purpose of Object.keys(store.purposeRoutes) as ModelPurpose[]) {
          if (store.purposeRoutes[purpose] === profileId) delete store.purposeRoutes[purpose];
        }
      }
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
    if (!apiKey && profile.authScheme !== "none") {
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
      profile.protocol,
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
      inferenceEndpoint: advanced.inferenceEndpoint,
      protocol: profile.protocol,
      authScheme: profile.authScheme,
      apiKey,
      temperature: advanced.temperature,
      maxTokens: advanced.maxTokens,
      contextWindow: advanced.contextWindow,
      maxInputTokens: advanced.maxInputTokens,
      promptCacheKey: advanced.promptCacheKey,
      credentialRevision,
      supportsVision: advanced.supportsVision,
      supportsNativePDF: advanced.supportsNativePDF,
      supportsVideo: advanced.supportsVideo,
      supportsTools: advanced.supportsTools,
      supportsPromptCache: advanced.supportsPromptCache,
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
    const protocol = request.protocol?.trim().toLowerCase() || "openai_chat";
    const inferenceEndpoint = request.advanced?.inferenceEndpoint?.trim() || "";
    return `profile_${createHash("sha256")
      .update(`${providerId}\0${baseURL}\0${modelId}\0${protocol}\0${inferenceEndpoint}`)
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
    const apiKey = request.apiKey?.trim() ?? "";
    const authScheme = normalizeAuthScheme(provider, request.authScheme);
    const protocol = normalizeProtocol(provider, request.protocol);
    if (!apiKey && authScheme !== "none") {
      return { ok: false, message: "连接测试失败：请先填写 API key。" };
    }
    const baseURL = normalizeProviderBaseURL(provider, request.baseURL);
    const modelId = provider.endpoint_model
      ? request.advanced?.endpointId?.trim() || ""
      : normalizeProviderModelId(provider, request.modelId);
    const requestId = `connection_${randomUUID().replaceAll("-", "")}`;
    const requestBody = connectionTestBody(provider, protocol, modelId);
    try {
      validateProtocolEndpointContract(
        provider,
        protocol,
        baseURL,
        request.advanced?.inferenceEndpoint,
      );
    } catch (error) {
      return {
        ok: false,
        message: error instanceof Error ? error.message : "连接测试失败：协议端点不兼容。",
      };
    }
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
      protocol_family: protocol,
      request_shape_digest: requestShapeDigest,
      max_output_tokens: protocol === "anthropic_messages" ? 64 : undefined,
      created_at: new Date(startedAt).toISOString(),
    });
    const controller = new AbortController();
    const timeout = setTimeout(
      () => controller.abort(),
      CONNECTION_TEST_TIMEOUT_MS,
    );
    try {
      const response = await testModelEndpoint(
        provider,
        baseURL,
        request.advanced?.inferenceEndpoint,
        protocol,
        authScheme,
        modelId,
        apiKey,
        controller.signal,
      );
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
        protocol_family: protocol,
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
        rate_limit_observation: rateLimitObservation(response.headers),
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
        return { ok: true, message: providerTestSuccessMessage(provider, protocol) };
      }
      return { ok: false, message: providerTestStatusMessage(response.status, provider, parsedBody) };
    } catch (error) {
      await this.appendConnectionUsage(usageScope, {
        schema_version: 1,
        kind: "model_usage",
        request_id: requestId,
        purpose: "connection_test",
        provider_id: provider.id,
        model_id: modelId,
        protocol_family: protocol,
        usage_complete: false,
        latency_ms: Date.now() - startedAt,
        status: error instanceof Error && error.name === "AbortError" ? "cancelled" : "failed",
        error_code: error instanceof Error && error.name === "AbortError" ? "timeout" : "transport_error",
        completed_at: new Date().toISOString(),
      });
      if (error instanceof Error && error.name === "AbortError") {
        return { ok: false, message: "连接测试超时：30 秒内未收到服务响应。" };
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
              max_tokens: 64,
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
    const localUsageByProvider = new Map<
      string,
      Extract<ModelProfileBalanceWire["detail"], { kind: "usage" }>
    >();

    const profilesByProvider = new Map<string, StoredModelProfile[]>();
    for (const profile of candidateProfiles) {
      const profiles = profilesByProvider.get(profile.providerId) ?? [];
      profiles.push(profile);
      profilesByProvider.set(profile.providerId, profiles);
    }
    await Promise.all(Array.from(profilesByProvider.entries()).map(async ([providerId, profiles]) => {
      const consoleURL = providerId === "openai"
        ? "https://platform.openai.com/usage"
        : providerId === "anthropic"
          ? "https://console.anthropic.com/settings/usage"
          : providerId === "google"
            ? "https://aistudio.google.com/usage"
            : "";
      const usage = await this.localUsageDetail(profiles.map((profile) => profile.id), consoleURL);
      if (usage?.kind === "usage") localUsageByProvider.set(providerId, usage);
    }));

    for (const profile of candidateProfiles) {
      if (providerKeys.has(profile.providerId)) continue;
      const provider = providerById(profile.providerId);
      if (!provider) continue;
      if (!isBalanceSupportedProvider(profile.providerId)) continue;
      if (profile.providerId === "google") continue;
      let apiKey = "";
      try {
        apiKey =
          (profile.providerId === "openai" || profile.providerId === "anthropic") &&
          this.envStore.getManagementKey
            ? await this.envStore.getManagementKey(profile.providerId)
            : await this.readProfileKey(profile);
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
      if (!apiKey) {
        continue;
      }
      providerKeys.set(profile.providerId, apiKey);
      providerKeyProfileIds.set(profile.providerId, profile.id);
      providerBaseURLs.set(
        profile.providerId,
        normalizeProviderBaseURL(provider, profile.baseURL),
      );
    }

    await Promise.allSettled(
      Array.from(providerKeys.entries()).map(async ([providerId, apiKey]) => {
        if (providerId === "google") return;
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
      const localUsage = localUsageByProvider.get(profile.providerId);

      if (profile.providerId === "google") {
        balances.push({
          profile_id: representative.id,
          provider_id: profile.providerId,
          model_id: modelId,
          display_name: displayName,
          status: "unsupported",
          error: "Google 普通 API Key 没有公开余额接口；下方展示 Awaken 本地逐次用量。",
          local_usage: localUsage,
        });
        continue;
      }

      if (!isBalanceSupportedProvider(profile.providerId)) {
        balances.push({
          profile_id: representative.id,
          provider_id: profile.providerId,
          model_id: modelId,
          display_name: displayName,
          status: "unsupported",
          error: "暂不支持查询该供应商余额",
          local_usage: localUsage,
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
          local_usage: localUsage,
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
          error:
            profile.providerId === "openai" || profile.providerId === "anthropic"
              ? "请先配置独立的组织用量管理密钥"
              : "请先配置 API key",
          local_usage: localUsage,
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
        local_usage: localUsage,
      });
    }

    return balances;
  }

  private async localUsageDetail(
    profileIds: string[],
    consoleURL: string,
  ): Promise<ModelProfileBalanceWire["detail"]> {
    let records: Array<Record<string, unknown>> = [];
    for (const profileId of profileIds) {
      const filePath = path.join(this.usageDir, `${profileId.replace(/[^A-Za-z0-9._-]/g, "_")}.jsonl`);
      try {
        records.push(...(await fs.readFile(filePath, "utf8"))
          .split(/\r?\n/u)
          .filter(Boolean)
          .flatMap((line) => {
            try { return [JSON.parse(line) as Record<string, unknown>]; } catch { return []; }
          }));
      } catch {
        // A profile without local usage contributes zero metrics.
      }
    }
    const profileSet = new Set(profileIds);
    try {
      const generation = (await fs.readFile(path.join(path.dirname(this.usageDir), "..", "CURRENT"), "utf8")).trim();
      if (/^[A-Za-z0-9._-]+$/.test(generation)) {
        const sessionsDir = path.join(path.dirname(this.usageDir), "..", "store-generations", generation, "sessions");
        const sessions = await fs.readdir(sessionsDir, { withFileTypes: true });
        for (const session of sessions) {
          if (!session.isDirectory()) continue;
          try {
            const journal = await fs.readFile(path.join(sessionsDir, session.name, "journal.jsonl"), "utf8");
            for (const line of journal.split(/\r?\n/u)) {
              if (!line) continue;
              try {
                const semantic = JSON.parse(line) as Record<string, unknown>;
                const payload = semantic.payload;
                if (semantic.kind !== "model_usage" || !payload || typeof payload !== "object") continue;
                const usage = payload as Record<string, unknown>;
                if (profileSet.has(String(usage.profile_id ?? ""))) {
                  records.push({ kind: "model_usage", ...usage });
                }
              } catch {
                // Ignore a torn final journal line; Core owns corruption recovery.
              }
            }
          } catch {
            // A session may disappear between directory listing and reading.
          }
        }
      }
    } catch {
      // Local usage is optional before the FileStore generation exists.
    }
    const usage = records.filter((record) => record.kind === "model_usage" && record.status === "succeeded");
    const sum = (field: string) => usage.reduce(
      (total, record) => total + (typeof record[field] === "number" ? record[field] as number : 0),
      0,
    );
    const latestRateLimit = [...records].reverse().find(
      (record) => record.rate_limit_observation && typeof record.rate_limit_observation === "object",
    )?.rate_limit_observation as Record<string, unknown> | undefined;
    const rateMetrics: ModelUsageMetricWire[] = latestRateLimit
      ? [
          ["最近一次请求剩余额度", latestRateLimit.request_remaining, "requests"] as const,
          ["最近一次 Token 剩余额度", latestRateLimit.token_remaining, "tokens"] as const,
        ].flatMap(([name, raw, unit]): ModelUsageMetricWire[] => {
          const value = typeof raw === "string" ? Number(raw) : raw;
          return typeof value === "number" && Number.isFinite(value)
            ? [{ name: `${name}（${String(latestRateLimit.observed_at ?? "未知时间")}）`, value, unit }]
            : [];
        })
      : [];
    return {
      kind: "usage",
      metrics: [
        { name: "Awaken 本地输入", value: sum("input_tokens"), unit: "tokens" },
        { name: "Awaken 本地缓存输入", value: sum("cached_input_tokens"), unit: "tokens" },
        { name: "Awaken 本地输出", value: sum("output_tokens"), unit: "tokens" },
        { name: "Awaken 已记录请求", value: usage.length, unit: "requests" },
        { name: "本地可计算成本", value: sum("calculated_cost_usd"), unit: "usd" },
        ...rateMetrics,
      ],
      console_url: consoleURL,
    };
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

  private async restoreManagementKey(providerId: string, previousKey: string): Promise<void> {
    if (previousKey) {
      await this.envStore.setManagementKey?.(providerId, previousKey);
    } else {
      await this.envStore.deleteManagementKey?.(providerId);
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
    const managementKey =
      (profile.providerId === "openai" || profile.providerId === "anthropic") &&
      this.envStore.getManagementKey
        ? await this.envStore.getManagementKey(profile.providerId).catch(() => "")
        : "";
    return toSummary(
      profile,
      apiKey !== "",
      this.observedCredentialStates.get(profile.id),
      effectiveCredentialRevision,
      resolution,
      managementKey,
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
        version: parsed.version === 3 ? 3 : parsed.version === 2 ? 2 : 1,
        profiles: Array.isArray(parsed.profiles) ? parsed.profiles : [],
        purposeRoutes:
          parsed.purposeRoutes && typeof parsed.purposeRoutes === "object"
            ? parsed.purposeRoutes
            : {},
      };
      const normalized = this.normalizeStore(store);
      if (parsed.version !== 3) {
        await this.writeStore(normalized);
      }
      return normalized;
    } catch (error) {
      if ((error as NodeJS.ErrnoException)?.code === "ENOENT") {
        return { version: 3, profiles: [], purposeRoutes: {} };
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
      version: 3,
      profiles: dedupeStoredProfiles(store.profiles.map(normalizeStoredProfile)),
      purposeRoutes: { ...(store.purposeRoutes ?? {}) },
    };
    const preferredProfileId =
      normalizedStore.profiles.find((profile) => profile.enabled && profile.isDefault)?.id ??
      normalizedStore.profiles[0]?.id ??
      "";
    this.ensureSingleDefault(normalizedStore, preferredProfileId);
    return normalizedStore;
  }
}
