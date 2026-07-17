import { mkdir, mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ModelProfileStore,
  type ModelProfileKeyStore,
} from "./model-profile-store.js";
import { AwakenEnvStore, AwakenEnvStoreError } from "./awaken-env-store.js";

let configDir: string;

function modelsPath(): string {
  return join(configDir, "config", "models.json");
}

async function writeModels(value: string): Promise<void> {
  await mkdir(join(configDir, "config"), { recursive: true });
  await writeFile(modelsPath(), value, "utf8");
}

beforeEach(async () => {
  configDir = await mkdtemp(join(tmpdir(), "awaken-model-profile-"));
});

afterEach(async () => {
  await rm(configDir, { recursive: true, force: true });
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("ModelProfileStore", () => {
  it("exposes all three current Kimi Coding Plan model IDs", () => {
    const store = new ModelProfileStore(configDir);
    const kimi = store
      .listProviders()
      .find((provider) => provider.id === "kimi-coding-plan");

    expect(kimi).toMatchObject({
      base_url: "https://api.kimi.com/coding/v1",
      default_model: "kimi-for-coding",
      models: ["k3", "kimi-for-coding", "kimi-for-coding-highspeed"],
      docs_url: "https://www.kimi.com/code/docs/kimi-code/models.html",
    });
  });

  it("exposes Volcengine Coding Plan models as suggestions while allowing custom IDs", () => {
    const store = new ModelProfileStore(configDir);
    const volcengine = store.listProviders().find((provider) => provider.id === "volcengine");

    expect(volcengine).toMatchObject({
      display_name: "火山 Coding Plan",
      base_url: "https://ark.cn-beijing.volces.com/api/coding/v3",
      allow_custom_model: true,
    });
    expect(volcengine?.models).toEqual(
      expect.arrayContaining(["deepseek-v4-pro", "glm-5.2", "kimi-k2.7-code", "minimax-m3"]),
    );
  });

  it("returns an accurate Chinese message for an unauthorized provider test", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: false, status: 401 }) as Response),
    );

    const store = new ModelProfileStore(configDir);
    const result = await store.testProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "bad-key",
      enabled: true,
      isDefault: true,
    });

    expect(result).toEqual({
      ok: false,
      message: "连接测试失败：API key 无效或未授权。",
    });
  });

  it("explains Kimi Code model-plan permissions for an unauthorized test", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: false, status: 401 }) as Response),
    );

    const store = new ModelProfileStore(configDir);
    const result = await store.testProfile({
      providerId: "kimi-coding-plan",
      displayName: "Kimi Coding Plan",
      modelId: "k3",
      baseURL: "https://api.kimi.com/coding/v1",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "valid-but-under-tier-key",
      enabled: true,
      isDefault: true,
    });

    expect(result).toEqual({
      ok: false,
      message:
        "连接测试失败：API key 无效，或当前会员套餐无权访问所选 Kimi Code 模型（K3 需 Moderato 及以上，高速版需 Allegretto 及以上）。",
    });
  });

  it("stores provider capability overrides under ~/.awaken/cache", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        status: 200,
        json: async () => ({
          data: [{ id: "deepseek-v4-pro", context_window: 256_000 }],
        }),
      }) as Response),
    );
    const store = new ModelProfileStore(configDir);

    await expect(store.testProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    })).resolves.toMatchObject({ ok: true });

    const cached = JSON.parse(
      await readFile(join(configDir, "cache", "model-capabilities.json"), "utf8"),
    ) as { providers: Record<string, Record<string, { context_window?: number }>> };
    expect(cached.providers.deepseek?.["deepseek-v4-pro"]?.context_window).toBe(256_000);
  });

  it("normalizes the Kimi Code base URL before testing", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200 }) as Response);
    vi.stubGlobal("fetch", fetchMock);

    const store = new ModelProfileStore(configDir);
    const result = await store.testProfile({
      providerId: "kimi-coding-plan",
      displayName: "Kimi Coding Plan",
      modelId: "kimi-for-coding",
      baseURL: "https://api.kimi.com/coding",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    });

    expect(result).toEqual({
      ok: true,
      message: "连接测试通过：Kimi Code 已响应 chat/completions。",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.kimi.com/coding/v1/chat/completions",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"temperature":1'),
      }),
    );
  });

  it("tests the selected K3 model ID", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200 }) as Response);
    vi.stubGlobal("fetch", fetchMock);

    const store = new ModelProfileStore(configDir);
    await expect(
      store.testProfile({
        providerId: "kimi-coding-plan",
        displayName: "Kimi Coding Plan",
        modelId: "k3",
        baseURL: "https://api.kimi.com/coding/v1",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "test-key",
        enabled: true,
        isDefault: true,
      }),
    ).resolves.toMatchObject({ ok: true });

    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.kimi.com/coding/v1/chat/completions",
      expect.objectContaining({
        body: expect.stringContaining('"model":"k3"'),
      }),
    );
  });

  it("normalizes Volcengine Coding Plan base URL and model before testing", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200 }) as Response);
    vi.stubGlobal("fetch", fetchMock);

    const store = new ModelProfileStore(configDir);
    const result = await store.testProfile({
      providerId: "volcengine",
      displayName: "火山 Coding Plan",
      modelId: "doubao-seed-2.0-pro",
      baseURL: "https://ark.cn-beijing.volces.com/api/v3",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    });

    expect(result).toEqual({
      ok: true,
      message: "连接测试通过：火山 Coding Plan 已响应 chat/completions。",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions",
      expect.objectContaining({
        method: "POST",
        body: expect.stringContaining('"model":"doubao-seed-2.0-pro"'),
      }),
    );
  });

  it("resolves Kimi Code run profile with provider-required temperature", async () => {
    const store = new ModelProfileStore(configDir);
    await store.saveProfile({
      providerId: "kimi-coding-plan",
      displayName: "Kimi Coding Plan",
      modelId: "kimi-for-coding",
      baseURL: "https://api.kimi.com/coding/v1",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    });

    const profile = await store.resolveDefaultRunProfile();

    expect(profile).toMatchObject({
      providerId: "kimi-coding-plan",
      modelId: "kimi-for-coding",
      baseURL: "https://api.kimi.com/coding/v1",
      temperature: 1,
      maxTokens: 32 * 1024,
      contextWindow: 256 * 1024,
    });
  });

  it("resolves K3 with the full 1048576-token context window", async () => {
    const store = new ModelProfileStore(configDir);
    await store.saveProfile({
      providerId: "kimi-coding-plan",
      displayName: "Kimi Coding Plan",
      modelId: "k3",
      baseURL: "https://api.kimi.com/coding/v1",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    });

    await expect(store.resolveDefaultRunProfile()).resolves.toMatchObject({
      providerId: "kimi-coding-plan",
      modelId: "k3",
      contextWindow: 1024 * 1024,
      maxTokens: 32 * 1024,
      supportsVision: true,
      supportsVideo: true,
    });
  });

  it("stores non-secret profile data in models.json and the key only in .env", async () => {
    const store = new ModelProfileStore(configDir);

    const saved = await store.saveProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    });

    const rawModels = await readFile(modelsPath(), "utf8");
    const persisted = JSON.parse(rawModels) as {
      profiles: Array<{ keyLast4?: string; credential?: unknown; apiKey?: string }>;
    };
    expect(persisted.profiles[0]).toMatchObject({ keyLast4: "-key" });
    expect(persisted.profiles[0]?.credential).toBeUndefined();
    expect(persisted.profiles[0]?.apiKey).toBeUndefined();
    expect(rawModels).not.toContain("test-key");
    expect(await readFile(join(configDir, ".env"), "utf8")).toContain(
      `AWAKEN_API_KEY_${saved.id}="test-key"`,
    );
    expect((await stat(join(configDir, ".env"))).mode & 0o777).toBe(0o600);
  });

  it("does not persist profile metadata when .env key writing fails", async () => {
    const keyStore: ModelProfileKeyStore = {
      getKey: async () => "",
      setKey: async () => {
        throw new AwakenEnvStoreError("write_failed", "env write failed");
      },
      deleteKey: async () => undefined,
    };
    const store = new ModelProfileStore(configDir, keyStore);

    await expect(
      store.saveProfile({
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "never-persist-this-key",
        enabled: true,
        isDefault: true,
      }),
    ).rejects.toMatchObject({ code: "write_failed" });

    await expect(readFile(modelsPath(), "utf8")).rejects.toMatchObject({
      code: "ENOENT",
    });
  });

  it("keeps an existing profile and key unchanged when replacement .env writing fails", async () => {
    const store = new ModelProfileStore(configDir);
    const saved = await store.saveProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "old-key",
      enabled: true,
      isDefault: true,
    });
    const before = await readFile(modelsPath(), "utf8");
    const realKeyStore = new AwakenEnvStore(configDir);
    const failingKeyStore: ModelProfileKeyStore = {
      getKey: (profileId) => realKeyStore.getKey(profileId),
      setKey: async () => {
        throw new AwakenEnvStoreError("write_failed", "env write failed");
      },
      deleteKey: (profileId) => realKeyStore.deleteKey(profileId),
    };
    const failingStore = new ModelProfileStore(configDir, failingKeyStore);

    await expect(
      failingStore.saveProfile({
        id: saved.id,
        providerId: "deepseek",
        displayName: "DeepSeek updated",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com/v2",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "new-key",
        enabled: true,
        isDefault: true,
      }),
    ).rejects.toMatchObject({ code: "write_failed" });

    expect(await readFile(modelsPath(), "utf8")).toBe(before);
    await expect(realKeyStore.getKey(saved.id)).resolves.toBe("old-key");
  });

  it("never treats a corrupted global profile store as an empty writable store", async () => {
    const corrupted = '{"version":1,"profiles":[';
    await writeModels(corrupted);
    const store = new ModelProfileStore(configDir);

    await expect(
      store.saveProfile({
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "new-key",
        enabled: true,
        isDefault: true,
      }),
    ).rejects.toBeInstanceOf(SyntaxError);

    expect(await readFile(modelsPath(), "utf8")).toBe(corrupted);
  });

  it("does not import the retired model-profiles.json store", async () => {
    await writeFile(
      join(configDir, "model-profiles.json"),
      JSON.stringify({
        version: 1,
        profiles: [
          {
            id: "legacy-profile",
            providerId: "deepseek",
            displayName: "DeepSeek",
            modelId: "deepseek-v4-pro",
            baseURL: "https://api.deepseek.com",
            protocol: "openai_chat",
            authScheme: "bearer",
            enabled: true,
            isDefault: true,
          },
        ],
      }),
      "utf8",
    );
    const store = new ModelProfileStore(configDir);

    await expect(store.listProfiles()).resolves.toEqual([]);
    await expect(store.resolveDefaultRunProfile()).resolves.toBeUndefined();
  });

  it("marks a profile unreadable when .env cannot be read", async () => {
    const store = new ModelProfileStore(configDir);
    const saved = await store.saveProfile({
      providerId: "kimi-coding-plan",
      displayName: "Kimi Coding Plan",
      modelId: "kimi-for-coding",
      baseURL: "https://api.kimi.com/coding/v1",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    });
    const unreadableKeyStore: ModelProfileKeyStore = {
      getKey: async () => {
        throw new AwakenEnvStoreError("unreadable", "env unreadable");
      },
      setKey: async () => undefined,
      deleteKey: async () => undefined,
    };
    const unreadableStore = new ModelProfileStore(configDir, unreadableKeyStore);

    await expect(unreadableStore.resolveDefaultRunProfile()).rejects.toMatchObject({
      name: "AwakenEnvStoreError",
      code: "unreadable",
    });
    await expect(unreadableStore.listProfiles()).resolves.toEqual([
      expect.objectContaining({
        id: saved.id,
        has_key: false,
        profile_status: "unreadable",
      }),
    ]);
  });

  it("normalizes an existing Volcengine Coding Plan profile to the coding gateway", async () => {
    await writeModels(
      JSON.stringify({
        version: 1,
        profiles: [
          {
            id: "volc-profile",
            providerId: "volcengine",
            displayName: "火山引擎",
            modelId: "doubao-seed-2.0-pro",
            baseURL: "https://ark.cn-beijing.volces.com/api/v3",
            protocol: "openai_chat",
            authScheme: "bearer",
            enabled: true,
            isDefault: true,
            advanced: {
              endpointId: "doubao-seed-2.0-pro",
              contextWindow: 1_000_000,
              maxTokens: 384_000,
            },
            keyLast4: "test",
          },
        ],
      }),
    );
    await new AwakenEnvStore(configDir).setKey("volc-profile", "api-test");

    const store = new ModelProfileStore(configDir);
    const profile = await store.resolveDefaultRunProfile();

    expect(profile).toMatchObject({
      providerId: "volcengine",
      modelId: "doubao-seed-2.0-pro",
      baseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
      maxTokens: 128_000,
      contextWindow: 256_000,
    });
  });

  it("preserves custom Volcengine Coding Plan model names", async () => {
    const store = new ModelProfileStore(configDir);
    await store.saveProfile({
      providerId: "volcengine",
      displayName: "火山 Coding Plan",
      modelId: "new-coding-plan-model",
      baseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
      enabled: true,
      isDefault: true,
    });

    const profile = await store.resolveDefaultRunProfile();

    expect(profile).toMatchObject({
      providerId: "volcengine",
      modelId: "new-coding-plan-model",
      baseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
      maxTokens: 32_000,
      contextWindow: 256_000,
    });
  });

  it("rejects a duplicate provider-model pair when adding a new profile", async () => {
    const store = new ModelProfileStore(configDir);

    await store.saveProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "first-key",
      enabled: true,
      isDefault: true,
    });

    await expect(
      store.saveProfile({
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com/v2",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "second-key",
        enabled: true,
        isDefault: true,
      }),
    ).rejects.toThrow("模型配置重复，添加失败");

    const profiles = await store.listProfiles();

    expect(profiles).toHaveLength(1);
    expect(profiles[0]).toMatchObject({
      model_id: "deepseek-v4-pro",
      base_url: "https://api.deepseek.com",
    });
  });

  it("allows updating an existing provider-model pair when the same profile id is provided", async () => {
    const store = new ModelProfileStore(configDir);

    const firstSave = await store.saveProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "first-key",
      enabled: true,
      isDefault: true,
    });

    const updatedSave = await store.saveProfile({
      id: firstSave.id,
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com/v2",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "second-key",
      enabled: true,
      isDefault: true,
    });

    const profiles = await store.listProfiles();

    expect(updatedSave.id).toBe(firstSave.id);
    expect(profiles).toHaveLength(1);
    expect(profiles[0]).toMatchObject({
      id: firstSave.id,
      model_id: "deepseek-v4-pro",
      base_url: "https://api.deepseek.com/v2",
    });
  });

  it("copies the template profile key when adding another model for the same provider", async () => {
    const store = new ModelProfileStore(configDir);

    const templateProfile = await store.saveProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "shared-key",
      enabled: true,
      isDefault: true,
    });

    const addedProfile = await store.saveProfile({
      templateProfileId: templateProfile.id,
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-flash",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      enabled: true,
      isDefault: false,
    });

    const profiles = await store.listProfiles();

    expect(addedProfile.id).not.toBe(templateProfile.id);
    expect(profiles).toHaveLength(2);
    expect(profiles.find((profile) => profile.id === addedProfile.id)).toMatchObject({
      model_id: "deepseek-v4-flash",
      has_key: true,
      masked_key: "•••• -key",
      is_default: false,
    });
  });

  it("reuses the template profile key during connection testing", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200 }) as Response);
    vi.stubGlobal("fetch", fetchMock);

    const store = new ModelProfileStore(configDir);
    const templateProfile = await store.saveProfile({
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "shared-key",
      enabled: true,
      isDefault: true,
    });

    const result = await store.testProfile({
      templateProfileId: templateProfile.id,
      providerId: "deepseek",
      displayName: "DeepSeek",
      modelId: "deepseek-v4-flash",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      enabled: true,
      isDefault: false,
    });

    expect(result).toEqual({
      ok: true,
      message: "连接测试通过：服务已响应 /models。",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.deepseek.com/models",
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: "Bearer shared-key",
        }),
      }),
    );
  });

  it("hides existing duplicate provider-model entries when listing profiles", async () => {
    await writeModels(
      JSON.stringify({
        version: 1,
        profiles: [
          {
            id: "deepseek-old",
            providerId: "deepseek",
            displayName: "DeepSeek",
            modelId: "deepseek-v4-pro",
            baseURL: "https://api.deepseek.com",
            protocol: "openai_chat",
            authScheme: "bearer",
            enabled: true,
            isDefault: true,
            advanced: {
              contextWindow: 128_000,
              maxTokens: 16_000,
            },
          },
          {
            id: "deepseek-new",
            providerId: "deepseek",
            displayName: "DeepSeek",
            modelId: "deepseek-v4-pro",
            baseURL: "https://api.deepseek.com/v2",
            protocol: "openai_chat",
            authScheme: "bearer",
            enabled: true,
            isDefault: false,
            advanced: {
              contextWindow: 256_000,
              maxTokens: 32_000,
            },
            keyLast4: "new1",
          },
        ],
      }),
    );
    await new AwakenEnvStore(configDir).setKey("deepseek-new", "key-new1");

    const store = new ModelProfileStore(configDir);
    const profiles = await store.listProfiles();

    expect(profiles).toHaveLength(1);
    expect(profiles[0]).toMatchObject({
      id: "deepseek-new",
      model_id: "deepseek-v4-pro",
      base_url: "https://api.deepseek.com/v2",
      has_key: true,
      masked_key: "•••• new1",
    });
  });

  describe("fetchBalances", () => {
    it("returns unsupported status for providers without balance support", async () => {
      const store = new ModelProfileStore(configDir);
      await store.saveProfile({
        providerId: "kimi-cn",
        displayName: "Kimi CN",
        modelId: "kimi-k2.6",
        baseURL: "https://api.moonshot.cn/v1",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "sk-test",
        enabled: true,
        isDefault: true,
      });

      const result = await store.fetchBalances();

      expect(result).toHaveLength(1);
      expect(result[0]).toMatchObject({
        provider_id: "kimi-cn",
        status: "unsupported",
      });
    });

    it("returns missing_key status for profiles without a .env key", async () => {
      const store = new ModelProfileStore(configDir);
      await store.saveProfile({
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        enabled: true,
        isDefault: true,
      });

      const result = await store.fetchBalances();

      expect(result).toHaveLength(1);
      expect(result[0]).toMatchObject({
        provider_id: "deepseek",
        status: "missing_key",
      });
    });

    it("fetches DeepSeek balance for a ready profile", async () => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () =>
          Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({
              is_available: true,
              balance_infos: [
                {
                  currency: "USD",
                  total_balance: "50.00",
                  granted_balance: "10.00",
                  topped_up_balance: "40.00",
                },
              ],
            }),
          } as Response),
        ),
      );

      const store = new ModelProfileStore(configDir);
      await store.saveProfile({
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "sk-test",
        enabled: true,
        isDefault: true,
      });

      const result = await store.fetchBalances();

      expect(result).toHaveLength(1);
      expect(result[0]).toMatchObject({
        provider_id: "deepseek",
        model_id: "deepseek-v4-pro",
        status: "available",
      });
      expect(result[0]?.detail).toEqual({
        kind: "amount",
        amounts: [{ currency: "USD", total: 50, granted: 10, topped_up: 40 }],
      });
    });

    it("deduplicates provider queries and returns one balance entry per provider", async () => {
      const fetchMock = vi.fn(async (input: Parameters<typeof fetch>[0]) => {
        const url = String(input);
        if (url.includes("/user/balance")) {
          return {
            ok: true,
            status: 200,
            json: async () => ({
              is_available: true,
              balance_infos: [
                {
                  currency: "USD",
                  total_balance: "30.00",
                  granted_balance: "0.00",
                  topped_up_balance: "30.00",
                },
              ],
            }),
          } as Response;
        }
        return {
          ok: true,
          status: 200,
          json: async () => ({
            usage: {
              limit: "100",
              used: "69",
              remaining: "31",
              resetTime: "2026-01-10T00:00:00.000Z",
            },
            limits: [
              {
                window: { duration: 300, timeUnit: "TIME_UNIT_MINUTE" },
                detail: {
                  limit: "100",
                  used: "25",
                  remaining: "75",
                  resetTime: "2026-01-06T04:00:00.000Z",
                },
              },
            ],
          }),
        } as Response;
      });
      vi.stubGlobal("fetch", fetchMock);

      const store = new ModelProfileStore(configDir);
      await store.saveProfile({
        providerId: "deepseek",
        displayName: "DeepSeek Pro",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "sk-test",
        enabled: true,
        isDefault: true,
      });
      await store.saveProfile({
        providerId: "deepseek",
        displayName: "DeepSeek Flash",
        modelId: "deepseek-v4-flash",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "sk-test-2",
        enabled: true,
        isDefault: false,
      });
      await store.saveProfile({
        providerId: "kimi-coding-plan",
        displayName: "Kimi Coding A",
        modelId: "kimi-for-coding",
        baseURL: "https://api.kimi.com/coding/v1",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "sk-kimi-1",
        enabled: true,
        isDefault: false,
      });
      await store.saveProfile({
        providerId: "kimi-coding-plan",
        displayName: "Kimi Coding B",
        modelId: "kimi-for-coding-highspeed",
        baseURL: "https://api.kimi.com/coding/v1",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "sk-kimi-2",
        enabled: true,
        isDefault: false,
      });

      const result = await store.fetchBalances();

      expect(result).toHaveLength(2);
      expect(result.map((entry) => entry.provider_id)).toEqual([
        "deepseek",
        "kimi-coding-plan",
      ]);
      expect(result.map((entry) => entry.display_name)).toEqual([
        "DeepSeek",
        "Kimi Coding Plan",
      ]);
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });
  });
});
