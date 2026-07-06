import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ModelProfileStore } from "./model-profile-store.js";

const electronMock = vi.hoisted(() => ({
  safeStorage: {
    decryptString: vi.fn(() => ""),
    encryptString: vi.fn(() => Buffer.from("encrypted")),
    isEncryptionAvailable: vi.fn(() => true),
  },
}));

vi.mock("electron", () => electronMock);

let configDir: string;

beforeEach(async () => {
  configDir = await mkdtemp(join(tmpdir(), "awaken-model-profile-"));
});

afterEach(async () => {
  await rm(configDir, { recursive: true, force: true });
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("ModelProfileStore", () => {
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

  it("migrates an existing Volcengine Coding Plan profile to the coding gateway", async () => {
    await writeFile(
      join(configDir, "model-profiles.json"),
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
            credential: {
              encryptedApiKey: Buffer.from("encrypted").toString("base64"),
              last4: "test",
            },
          },
        ],
      }),
      "utf8",
    );

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

  it("copies the template profile credential when adding another model for the same provider", async () => {
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

  it("reuses the template profile credential during connection testing", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200 }) as Response);
    vi.stubGlobal("fetch", fetchMock);
    electronMock.safeStorage.decryptString.mockReturnValue("shared-key");

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
    await writeFile(
      join(configDir, "model-profiles.json"),
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
            credential: {
              encryptedApiKey: Buffer.from("old").toString("base64"),
              last4: "old1",
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
          },
        ],
      }),
      "utf8",
    );

    const store = new ModelProfileStore(configDir);
    const profiles = await store.listProfiles();

    expect(profiles).toHaveLength(1);
    expect(profiles[0]).toMatchObject({
      id: "deepseek-new",
      model_id: "deepseek-v4-pro",
      base_url: "https://api.deepseek.com/v2",
      has_key: true,
      masked_key: "•••• old1",
    });
  });
});
