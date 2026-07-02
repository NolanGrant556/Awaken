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
  withModelCapabilities,
} from "./model-capabilities.js";

const MODEL_PROFILES_FILE = "model-profiles.json";

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
    base_url: "https://api.moonshot.cn/v1",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "MOONSHOT_API_KEY",
    default_model: "kimi-k2.7-code",
    models: ["kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6"],
    docs_url: "https://platform.kimi.com/docs/guide/kimi-k2-7-code-quickstart",
    api_docs_url: "https://platform.kimi.com/console/api-keys",
    coding_preset: true,
  },
  {
    id: "volcengine",
    display_name: "火山引擎",
    base_url: "https://ark.cn-beijing.volces.com/api/v3",
    protocol: "openai_chat",
    auth_scheme: "bearer",
    key_env_name: "ARK_API_KEY",
    default_model: "ep-your-ark-endpoint-id",
    models: ["ep-your-ark-endpoint-id"],
    docs_url: "https://www.volcengine.com/docs/82379/1330626",
    api_docs_url: "https://www.volcengine.com/docs/82379/1541594",
    endpoint_model: true,
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

function toSummary(profile: StoredModelProfile): RuntimeModelProfileSummaryWire {
  const advanced = normalizeAdvancedWithModelCapability(
    profile.advanced,
    profile.providerId,
    profile.advanced?.endpointId?.trim() || profile.modelId,
  );
  return {
    id: profile.id,
    provider_id: profile.providerId,
    display_name: profile.displayName,
    model_id: profile.modelId,
    base_url: profile.baseURL,
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

export class ModelProfileStore {
  private filePath: string;

  constructor(configDir: string) {
    this.filePath = path.join(configDir, MODEL_PROFILES_FILE);
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
      throw new Error("Unknown model provider");
    }

    const store = await this.readStore();
    const existingIndex = request.id
      ? store.profiles.findIndex((profile) => profile.id === request.id)
      : -1;
    const existing = existingIndex >= 0 ? store.profiles[existingIndex] : undefined;
    const modelId = request.modelId.trim() || provider.default_model;
    const advanced = normalizeAdvancedWithModelCapability(
      request.advanced ?? existing?.advanced,
      provider.id,
      request.advanced?.endpointId?.trim() || modelId,
    );
    const profile: StoredModelProfile = {
      id: existing?.id ?? randomUUID(),
      providerId: provider.id,
      displayName: request.displayName?.trim() || provider.display_name,
      modelId,
      baseURL: trimTrailingSlash(request.baseURL || provider.base_url),
      protocol: request.protocol?.trim() || provider.protocol,
      authScheme: request.authScheme?.trim() || provider.auth_scheme,
      enabled: request.enabled ?? existing?.enabled ?? true,
      isDefault: request.isDefault ?? existing?.isDefault ?? store.profiles.length === 0,
      advanced,
      credential: existing?.credential,
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

    const advanced = normalizeAdvancedWithModelCapability(
      profile.advanced,
      profile.providerId,
      profile.advanced?.endpointId?.trim() || profile.modelId,
    );
    return {
      profileId: profile.id,
      providerId: profile.providerId,
      displayName: profile.displayName,
      modelId: advanced.endpointId?.trim() || profile.modelId,
      baseURL: profile.baseURL,
      protocol: profile.protocol,
      authScheme: profile.authScheme,
      apiKey: decryptCredential(profile.credential),
      temperature: advanced.temperature,
      maxTokens: advanced.maxTokens,
      contextWindow: advanced.contextWindow,
      promptCacheKey: advanced.promptCacheKey,
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
      return { ok: false, message: "Unknown model provider" };
    }
    const apiKey = request.apiKey?.trim() || (request.id ? await this.apiKeyForProfile(request.id) : "");
    if (!apiKey) {
      return { ok: false, message: "API key is required before testing" };
    }
    const baseURL = trimTrailingSlash(request.baseURL || provider.base_url);
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 8000);
    try {
      const response = await fetch(`${baseURL}/models`, {
        method: "GET",
        headers: {
          Authorization: `Bearer ${apiKey}`,
        },
        signal: controller.signal,
      });
      if (response.ok) {
        return { ok: true, message: "Provider responded successfully" };
      }
      return { ok: false, message: `Provider test failed with status ${response.status}` };
    } catch {
      return { ok: false, message: "Provider test failed before a successful response" };
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
      return {
        version: 1,
        profiles: Array.isArray(parsed.profiles) ? parsed.profiles : [],
      };
    } catch {
      return { version: 1, profiles: [] };
    }
  }

  private async writeStore(store: StoreFile): Promise<void> {
    await fs.mkdir(path.dirname(this.filePath), { recursive: true });
    const tempPath = `${this.filePath}.${process.pid}.tmp`;
    await fs.writeFile(tempPath, `${JSON.stringify(store, null, 2)}\n`, "utf8");
    await fs.rename(tempPath, this.filePath);
  }
}
