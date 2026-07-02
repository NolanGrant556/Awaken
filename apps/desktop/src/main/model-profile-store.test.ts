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
});
