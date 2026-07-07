import { randomUUID } from "node:crypto";
import fs from "node:fs/promises";
import path from "node:path";
import { safeStorage } from "electron";
import type {
  ModelProfileAdvanced,
  ModelProfileSaveRequest,
  ModelProfileTestResult,
  ModelProviderWire,
  RuntimeModelProfileSummaryWire,
  ModelProfileRunConfig,
} from "../shared/runtime.js";
import {
  normalizeAdvancedWithModelCapability,
  setCapabilityOverrides,
  withModelCapabilities,
} from "./model-capabilities.js";
import {
  fetchProviderCapabilities,
  loadCapabilityOverrides,
  mergeCapabilityOverrides,
  parseProviderCapabilities,
  saveCapabilityOverrides,
  type ProviderCapabilityOverrides,
} from "./model-capability-sync.js";

const MODEL_PROFILES_FILE = "model-profiles.json";
const DUPLICATE_MODEL_PROFILE_ERROR = "模型配置重复，添加失败";
const CAPABILITY_SYNC_TIMEOUT_MS = 8_000;

export const BUILTIN_MODEL_PROVIDERS: ModelProviderWire[] = [
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
  },
  {
    id: "kimi-cn",
    display_name: "Kimi CN",
    base_url: "https://api.moonshot.cn/v1",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "MOONSHOT_API_KEY",
    default_model: "kimi-k2.6",
    models: [
      "kimi-k2.6",
      "kimi-k2.7-code",
      "kimi-k2.7-code-highspeed",
      "kimi-k2.5",
      "moonshot-v1-128k",
      "moonshot-v1-32k",
      "moonshot-v1-8k",
    ],
    docs_url: "https://platform.kimi.com/docs",
    api_docs_url: "https://platform.kimi.com/console/api-keys",
  },
  {
    id: "kimi-coding-plan",
    display_name: "Kimi Coding Plan",
    base_url: "https://api.kimi.com/coding/v1",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "KIMI_CODE_API_KEY",
    default_model: "kimi-for-coding",
    models: ["kimi-for-coding"],
    docs_url: "https://www.kimi.com/code/docs/en/third-party-tools/other-coding-agents.html",
    api_docs_url: "https://www.kimi.com/code/console",
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
    allow_custom_model: true,
  },
];

type StoredCredential = {
  encryptedApiKey: string;
  last4: string;
};

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
  credential?: StoredCredential;
};

type StoreFile = {
  version: 1;
  profiles: StoredModelProfile[];
};

function providerById(providerId: string): ModelProviderWire | undefined {
  return BUILTIN_MODEL_PROVIDERS.find((provider) => provider.id === providerId);
}

function trimTrailingSlash(value: string): string {
  return value.trim().replace(/\/+$/, "");
}

function normalizeProviderModelId(provider: ModelProviderWire | undefined, modelId: string): string {
  const trimmed = modelId.trim();
  if (!provider) return trimmed;
  if (provider.endpoint_model) return trimmed || provider.default_model;
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
  return normalized;
}

function maskKey(credential?: StoredCredential): string | undefined {
  if (!credential?.last4) {
    return undefined;
  }
  return `•••• ${credential.last4}`;
}

function profileStatus(profile: StoredModelProfile): RuntimeModelProfileSummaryWire["profile_status"] {
  if (!profile.enabled) return "disabled";
  if (!profile.credential) return "missing_key";
  return "ready";
}

function providerTestStatusMessage(status: number, provider?: ModelProviderWire): string {
  if (provider?.id === "kimi-coding-plan") {
    switch (status) {
      case 402:
        return "连接测试失败：Kimi Code 会员权益无法验证，请确认会员状态。";
      case 403:
        return "连接测试失败：Kimi Code 拒绝当前客户端或账号权限，请确认会员权益、额度和客户端支持状态。";
      case 404:
        return "连接测试失败：未找到 kimi-for-coding 或账号无权访问该模型。";
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
  if (provider.id === "kimi-coding-plan" || provider.id === "volcengine") {
    return fetch(`${baseURL}/chat/completions`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${apiKey}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        model: modelId || provider.default_model,
        messages: [{ role: "user", content: "ping" }],
        ...(provider.id === "kimi-coding-plan" ? { temperature: 1 } : {}),
        max_tokens: 1,
        stream: false,
      }),
      signal,
    });
  }
  return fetch(`${baseURL}/models`, {
    method: "GET",
    headers: {
      Authorization: `Bearer ${apiKey}`,
    },
    signal,
  });
}

function providerTestSuccessMessage(provider: ModelProviderWire): string {
  if (provider.id === "kimi-coding-plan") {
    return "连接测试通过：Kimi Code 已响应 chat/completions。";
  }
  if (provider.id === "volcengine") {
    return "连接测试通过：火山 Coding Plan 已响应 chat/completions。";
  }
  return "连接测试通过：服务已响应 /models。";
}

function toSummary(profile: StoredModelProfile): RuntimeModelProfileSummaryWire {
  const provider = providerById(profile.providerId);
  const modelId = normalizeProviderModelId(provider, profile.modelId);
  const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
  const advanced = normalizeAdvancedWithModelCapability(
    profile.advanced,
    profile.providerId,
    provider?.endpoint_model ? profile.advanced?.endpointId?.trim() || modelId : modelId,
  );
  return {
    id: profile.id,
    provider_id: profile.providerId,
    display_name: profile.displayName,
    model_id: modelId,
    base_url: baseURL,
    protocol: profile.protocol,
    auth_scheme: profile.authScheme,
    has_key: Boolean(profile.credential),
    masked_key: maskKey(profile.credential),
    enabled: profile.enabled,
    is_default: profile.isDefault,
    profile_status: profileStatus(profile),
    advanced: {
      temperature: advanced.temperature,
      max_tokens: advanced.maxTokens,
      endpoint_id: advanced.endpointId,
      prompt_cache_key: advanced.promptCacheKey,
      context_window: advanced.contextWindow,
      supports_vision: advanced.supportsVision,
      supports_video: advanced.supportsVideo,
    },
  };
}

function assertSecureStorageAvailable(): void {
  if (!safeStorage.isEncryptionAvailable()) {
    throw new Error("Secure credential storage is unavailable on this device");
  }
}

function encryptedCredential(apiKey: string): StoredCredential {
  assertSecureStorageAvailable();
  const trimmed = apiKey.trim();
  const encrypted = safeStorage.encryptString(trimmed);
  return {
    encryptedApiKey: encrypted.toString("base64"),
    last4: trimmed.slice(-4),
  };
}

function decryptCredential(credential?: StoredCredential): string {
  if (!credential) return "";
  assertSecureStorageAvailable();
  return safeStorage.decryptString(Buffer.from(credential.encryptedApiKey, "base64"));
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
    credential: preferred.credential ?? fallback.credential,
    isDefault: preferred.isDefault || fallback.isDefault,
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
  private configDir: string;
  private capabilityOverrides: Record<string, ProviderCapabilityOverrides> = {};

  constructor(configDir: string) {
    this.filePath = path.join(configDir, MODEL_PROFILES_FILE);
    this.configDir = configDir;
  }

  async initialize(): Promise<void> {
    this.capabilityOverrides = await loadCapabilityOverrides(this.configDir);
    setCapabilityOverrides(this.capabilityOverrides);
  }

  async syncCapabilities(): Promise<Record<string, ProviderCapabilityOverrides>> {
    const store = await this.readStore();
    const candidates = new Map<
      string,
      { baseURL: string; apiKey: string }
    >();

    for (const profile of store.profiles) {
      if (!profile.enabled || !profile.credential) continue;
      if (candidates.has(profile.providerId)) continue;
      const provider = providerById(profile.providerId);
      if (!provider) continue;
      const apiKey = decryptCredential(profile.credential);
      if (!apiKey) continue;
      candidates.set(profile.providerId, {
        baseURL: normalizeProviderBaseURL(provider, profile.baseURL),
        apiKey,
      });
    }

    const fetched: Record<string, ProviderCapabilityOverrides> = {};
    await Promise.allSettled(
      Array.from(candidates.entries()).map(async ([providerId, { baseURL, apiKey }]) => {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), CAPABILITY_SYNC_TIMEOUT_MS);
        try {
          const overrides = await fetchProviderCapabilities(baseURL, apiKey, controller.signal);
          fetched[providerId] = overrides;
        } catch {
          // Ignore per-provider fetch failures; keep existing overrides.
        } finally {
          clearTimeout(timeout);
        }
      }),
    );

    this.capabilityOverrides = mergeCapabilityOverrides(this.capabilityOverrides, fetched);
    setCapabilityOverrides(this.capabilityOverrides);
    await saveCapabilityOverrides(this.configDir, this.capabilityOverrides);
    return this.capabilityOverrides;
  }

  listProviders(): ModelProviderWire[] {
    return BUILTIN_MODEL_PROVIDERS.map((provider) =>
      withModelCapabilities({
        ...provider,
        models: [...provider.models],
      }),
    );
  }

  async listProfiles(): Promise<RuntimeModelProfileSummaryWire[]> {
    const store = await this.readStore();
    return store.profiles.map(toSummary);
  }

  async saveProfile(request: ModelProfileSaveRequest): Promise<RuntimeModelProfileSummaryWire> {
    const provider = providerById(request.providerId);
    if (!provider) {
      throw new Error("未知模型供应商");
    }

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
    const advanced = normalizeAdvancedWithModelCapability(
      request.advanced ?? existing?.advanced ?? templateProfile?.advanced,
      provider.id,
      provider.endpoint_model ? request.advanced?.endpointId?.trim() || modelId : modelId,
    );
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
      credential: existing?.credential ?? templateProfile?.credential,
    };

    if (request.apiKey !== undefined && request.apiKey.trim() !== "") {
      profile.credential = encryptedCredential(request.apiKey);
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
    await this.writeStore(store);
    return toSummary(profile);
  }

  async deleteProfile(profileId: string): Promise<void> {
    const store = await this.readStore();
    const previousDefault = store.profiles.find((profile) => profile.id === profileId)?.isDefault;
    store.profiles = store.profiles.filter((profile) => profile.id !== profileId);
    if (previousDefault && store.profiles.length > 0) {
      store.profiles[0].isDefault = true;
    }
    await this.writeStore(store);
  }

  async setDefaultProfile(profileId: string): Promise<RuntimeModelProfileSummaryWire> {
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
    return toSummary(profile);
  }

  async resolveDefaultRunProfile(): Promise<ModelProfileRunConfig | undefined> {
    const store = await this.readStore();
    const profile =
      store.profiles.find((entry) => entry.isDefault && entry.enabled) ??
      store.profiles.find((entry) => entry.enabled);
    if (!profile) return undefined;
    if (!profile.credential) {
      throw new Error("Configure an API key in Settings > Models before running Awaken");
    }

    const provider = providerById(profile.providerId);
    const modelId = normalizeProviderModelId(provider, profile.modelId);
    const baseURL = normalizeProviderBaseURL(provider, profile.baseURL);
    const advanced = normalizeAdvancedWithModelCapability(
      profile.advanced,
      profile.providerId,
      provider?.endpoint_model ? profile.advanced?.endpointId?.trim() || modelId : modelId,
    );
    return {
      profileId: profile.id,
      providerId: profile.providerId,
      displayName: profile.displayName,
      modelId: provider?.endpoint_model ? advanced.endpointId?.trim() || modelId : modelId,
      baseURL,
      protocol: profile.protocol,
      authScheme: profile.authScheme,
      apiKey: decryptCredential(profile.credential),
      temperature: advanced.temperature,
      maxTokens: advanced.maxTokens,
      contextWindow: advanced.contextWindow,
      promptCacheKey: advanced.promptCacheKey,
      supportsVision: advanced.supportsVision,
      supportsVideo: advanced.supportsVideo,
    };
  }

  async defaultProfileSummary(): Promise<RuntimeModelProfileSummaryWire | undefined> {
    const store = await this.readStore();
    const profile =
      store.profiles.find((entry) => entry.isDefault) ??
      store.profiles.find((entry) => entry.enabled);
    return profile ? toSummary(profile) : undefined;
  }

  async testProfile(request: ModelProfileSaveRequest): Promise<ModelProfileTestResult> {
    const provider = providerById(request.providerId);
    if (!provider) {
      return { ok: false, message: "连接测试失败：未知模型供应商。" };
    }
    const apiKey =
      request.apiKey?.trim() ||
      (request.id
        ? await this.apiKeyForProfile(request.id)
        : request.templateProfileId
          ? await this.apiKeyForProfile(request.templateProfileId)
          : "");
    if (!apiKey) {
      return { ok: false, message: "连接测试失败：请先填写 API key。" };
    }
    const baseURL = normalizeProviderBaseURL(provider, request.baseURL);
    const modelId = normalizeProviderModelId(provider, request.modelId);
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 8000);
    try {
      const response = await testModelEndpoint(provider, baseURL, modelId, apiKey, controller.signal);
      if (response.ok) {
        if (provider.id !== "kimi-coding-plan" && provider.id !== "volcengine") {
          try {
            const overrides = parseProviderCapabilities(await response.json());
            this.capabilityOverrides = mergeCapabilityOverrides(
              this.capabilityOverrides,
              { [provider.id]: overrides },
            );
            setCapabilityOverrides(this.capabilityOverrides);
            await saveCapabilityOverrides(this.configDir, this.capabilityOverrides);
          } catch {
            // Ignore parse failures; the connection test itself succeeded.
          }
        }
        return { ok: true, message: providerTestSuccessMessage(provider) };
      }
      return { ok: false, message: providerTestStatusMessage(response.status, provider) };
    } catch (error) {
      if (error instanceof Error && error.name === "AbortError") {
        return { ok: false, message: "连接测试超时：8 秒内未收到服务响应。" };
      }
      return { ok: false, message: "连接测试失败：无法连接到服务，请检查 Base URL 或网络。" };
    } finally {
      clearTimeout(timeout);
    }
  }

  private async apiKeyForProfile(profileId: string): Promise<string> {
    const store = await this.readStore();
    const profile = store.profiles.find((entry) => entry.id === profileId);
    return decryptCredential(profile?.credential);
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
        version: 1,
        profiles: Array.isArray(parsed.profiles) ? parsed.profiles : [],
      };
      return this.normalizeStore(store);
    } catch {
      return { version: 1, profiles: [] };
    }
  }

  private async writeStore(store: StoreFile): Promise<void> {
    const normalizedStore = this.normalizeStore(store);
    await fs.mkdir(path.dirname(this.filePath), { recursive: true });
    const tempPath = `${this.filePath}.${process.pid}.tmp`;
    await fs.writeFile(tempPath, `${JSON.stringify(normalizedStore, null, 2)}\n`, "utf8");
    await fs.rename(tempPath, this.filePath);
  }

  private normalizeStore(store: StoreFile): StoreFile {
    const normalizedStore: StoreFile = {
      version: 1,
      profiles: dedupeStoredProfiles(store.profiles),
    };
    const preferredProfileId =
      normalizedStore.profiles.find((profile) => profile.enabled && profile.isDefault)?.id ??
      normalizedStore.profiles[0]?.id ??
      "";
    this.ensureSingleDefault(normalizedStore, preferredProfileId);
    return normalizedStore;
  }
}
