import {
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ModelProfileStore,
  type ModelProfileKeyStore,
} from "./model-profile-store.js";
import { AwakenEnvStore, AwakenEnvStoreError } from "./awaken-env-store.js";

let configDir: string;
const THREE_LLM_PROTOCOLS = [
  "openai_chat",
  "openai_responses",
  "anthropic_messages",
];
const EXPECTED_PROVIDER_PROTOCOLS: Record<string, string[]> = {
  openai: ["openai_chat", "openai_responses"],
  anthropic: ["anthropic_messages"],
  google: ["openai_chat"],
  custom: ["openai_chat", "openai_responses", "anthropic_messages"],
  deepseek: ["openai_chat", "openai_responses", "anthropic_messages"],
  "kimi-cn": ["openai_chat"],
  "kimi-coding-plan": ["openai_chat", "anthropic_messages"],
  "minimax-token-plan": [
    "openai_chat",
    "openai_responses",
    "anthropic_messages",
  ],
  "glm-coding-plan": ["openai_chat", "anthropic_messages"],
  minimax: ["openai_chat", "openai_responses", "anthropic_messages"],
  glm: ["openai_chat", "anthropic_messages"],
  qwen: ["openai_chat", "openai_responses", "anthropic_messages"],
  doubao: ["openai_chat", "openai_responses"],
  "qwen-token-plan": [
    "openai_chat",
    "openai_responses",
    "anthropic_messages",
  ],
  volcengine: ["openai_chat", "anthropic_messages"],
  "volcengine-agent-plan": ["openai_chat", "anthropic_messages"],
};

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
  it("groups built-in entries into Coding Plan, API, Agent Plan, and Custom", () => {
    const providers = new ModelProfileStore(configDir).listProviders();
    expect(
      providers
        .filter((provider) => provider.access_type === "coding_plan")
        .map((provider) => provider.id),
    ).toEqual([
      "kimi-coding-plan",
      "minimax-token-plan",
      "glm-coding-plan",
      "volcengine",
      "qwen-token-plan",
    ]);
    expect(
      providers
        .filter((provider) => provider.access_type === "api")
        .map((provider) => provider.id),
    ).toEqual([
      "openai",
      "anthropic",
      "google",
      "kimi-cn",
      "minimax",
      "glm",
      "qwen",
      "doubao",
      "deepseek",
    ]);
    expect(
      providers
        .filter((provider) => provider.access_type === "agent_plan")
        .map((provider) => provider.id),
    ).toEqual(["volcengine-agent-plan"]);
    expect(
      providers
        .filter((provider) => provider.access_type === "custom")
        .map((provider) => provider.id),
    ).toEqual(["custom"]);
  });

  it("keeps every built-in provider on its precise protocol matrix", () => {
    const providers = new ModelProfileStore(configDir).listProviders();

    for (const provider of providers) {
      expect(provider.supported_protocols).toEqual(
        EXPECTED_PROVIDER_PROTOCOLS[provider.id],
      );
    }

    expect(providers.find((provider) => provider.id === "openai")?.catalog_strategy).toBe("openai");
    expect(providers.find((provider) => provider.id === "custom")?.catalog_strategy).toBe("none");
    expect(providers.find((provider) => provider.id === "minimax-token-plan")).toMatchObject({
      base_url: "https://api.minimaxi.com/v1",
      protocol_base_urls: {
        openai_chat: "https://api.minimaxi.com/v1",
        openai_responses: "https://api.minimaxi.com/v1",
        anthropic_messages: "https://api.minimaxi.com/anthropic",
      },
      catalog_url: "https://api.minimaxi.com/v1/models",
    });
    expect(providers.find((provider) => provider.id === "minimax")).toMatchObject({
      base_url: "https://api.minimaxi.com/v1",
      catalog_url: "https://api.minimaxi.com/v1/models",
    });
    expect(providers.find((provider) => provider.id === "qwen-token-plan")).toMatchObject({
      catalog_url:
        "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/models",
      allow_custom_model: true,
    });
    expect(providers.find((provider) => provider.id === "doubao")).toMatchObject({
      base_url: "https://ark.cn-beijing.volces.com/api/v3",
      default_model: "doubao-seed-2-1-pro-260628",
      models: expect.arrayContaining([
        "doubao-seed-evolving",
        "doubao-seed-2-1-pro-260628",
        "doubao-seed-2-1-turbo-260628",
        "doubao-seed-2-0-code-preview-260215",
        "doubao-seed-2-0-pro-260215",
        "doubao-seed-2-0-lite-260428",
      ]),
      model_display_names: {
        "doubao-seed-evolving": "Doubao-Seed-Evolving",
        "doubao-seed-2-1-pro-260628": "Doubao-Seed-2.1-Pro",
        "doubao-seed-2-1-turbo-260628": "Doubao-Seed-2.1-Turbo",
        "doubao-seed-2-0-code-preview-260215": "Doubao-Seed-2.0-Code",
        "doubao-seed-2-0-pro-260215": "Doubao-Seed-2.0-Pro",
        "doubao-seed-2-0-lite-260428": "Doubao-Seed-2.0-Lite",
      },
      protocol_base_urls: {
        openai_chat: "https://ark.cn-beijing.volces.com/api/v3",
        openai_responses: "https://ark.cn-beijing.volces.com/api/v3",
      },
      model_capabilities: {
        "doubao-seed-2-1-pro-260628": expect.objectContaining({
          supported_protocols: ["openai_chat", "openai_responses"],
        }),
      },
    });
  });

  it("migrates stored MiniMax international endpoints to the China service", async () => {
    await writeModels(JSON.stringify({
      version: 3,
      profiles: [
        {
          id: "minimax-chat",
          providerId: "minimax",
          displayName: "MiniMax",
          modelId: "MiniMax-M3",
          baseURL: "https://api.minimax.io/v1",
          protocol: "openai_chat",
          authScheme: "bearer",
          enabled: true,
          isDefault: true,
        },
        {
          id: "minimax-plan-messages",
          providerId: "minimax-token-plan",
          displayName: "MiniMax Token Plan",
          modelId: "MiniMax-M3",
          baseURL: "https://api.minimax.io/anthropic",
          protocol: "anthropic_messages",
          authScheme: "x-api-key",
          enabled: true,
          isDefault: false,
        },
      ],
    }));

    await expect(new ModelProfileStore(configDir).listProfiles()).resolves.toEqual([
      expect.objectContaining({ id: "minimax-chat", base_url: "https://api.minimaxi.com/v1" }),
      expect.objectContaining({
        id: "minimax-plan-messages",
        base_url: "https://api.minimaxi.com/anthropic",
      }),
    ]);
  });

  it("resolves the requested enabled profile instead of forcing the global default", async () => {
    const store = new ModelProfileStore(configDir);
    const first = await store.saveProfile({
      providerId: "deepseek",
      modelId: "deepseek-v4-pro",
      apiKey: "deepseek-key",
      isDefault: true,
    });
    const second = await store.saveProfile({
      providerId: "kimi-cn",
      modelId: "kimi-k3",
      apiKey: "kimi-key",
      isDefault: false,
    });

    expect((await store.resolveDefaultRunProfile())?.profileId).toBe(first.id);
    expect(await store.resolveRunProfile(second.id, "low")).toMatchObject({
      profileId: second.id,
      providerId: "kimi-cn",
      modelId: "kimi-k3",
      reasoningEffort: "low",
    });
    expect((await store.resolveRunProfile(first.id, "low"))?.profileId).toBe(first.id);
  });

  it("preserves explicit and unknown tool capability instead of forcing ready", async () => {
    const store = new ModelProfileStore(configDir);
    const profile = await store.saveProfile({
      providerId: "qwen",
      modelId: "custom-tool-model",
      apiKey: "test-key",
      advanced: { supportsTools: "unsupported" },
    });

    expect(
      store.listProviders().find((provider) => provider.id === "minimax")
        ?.model_capabilities?.["MiniMax-M3"]?.supports_tools,
    ).toBe("ready");
    await expect(store.resolveRunProfile(profile.id)).resolves.toMatchObject({
      supportsTools: undefined,
    });
  });

  it("keeps native PDF and cache capability scoped to the selected protocol contract", async () => {
    const store = new ModelProfileStore(configDir);
    const responses = await store.saveProfile({
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_responses",
      apiKey: "openai-key",
      isDefault: true,
    });
    const googleChat = await store.saveProfile({
      providerId: "google",
      modelId: "gemini-2.5-pro",
      protocol: "openai_chat",
      apiKey: "google-key",
    });

    await expect(store.resolveRunProfile(responses.id)).resolves.toMatchObject({
      protocol: "openai_responses",
      supportsNativePDF: true,
      supportsPromptCache: true,
    });
    await expect(store.resolveRunProfile(googleChat.id)).resolves.toMatchObject({
      protocol: "openai_chat",
      supportsNativePDF: false,
      supportsPromptCache: false,
    });
  });

  it("reveals Agent Plan effort only after an exact model accepts the controlled parameter", async () => {
    const fetchMock = vi.fn(
      async (url: string, _init?: RequestInit) =>
        ({
          ok: true,
          status: 200,
          json: async () => ({
            data: url.endsWith("/models") ? [{ id: "glm-5.2" }] : [],
          }),
        }) as Response,
    );
    vi.stubGlobal("fetch", fetchMock);
    const store = new ModelProfileStore(configDir);
    await store.saveAgentPlanAccount({ apiKey: "test-key" });
    const profile = await store.saveProfile({
      providerId: "volcengine-agent-plan",
      modelId: "glm-5.2",
    });

    await expect(
      store.testAgentPlanReasoningCapability({
        id: profile.id,
        providerId: "volcengine-agent-plan",
        modelId: "glm-5.2",
        baseURL: "https://ignored.example/v1",
        apiKey: "test-key",
      }),
    ).resolves.toMatchObject({
      ok: true,
      reasoningEffort: { levels: ["high", "max"], defaultLevel: "high" },
    });

    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(
      fetchMock.mock.calls
        .filter(([, init]) => init?.body)
        .map(([, init]) => JSON.parse((init as RequestInit).body as string)),
    ).toEqual([
      expect.objectContaining({ reasoning_effort: "high" }),
      expect.objectContaining({ reasoning_effort: "max" }),
    ]);
    expect(
      fetchMock.mock.calls.every(
        ([, init]) => (init as RequestInit).signal instanceof AbortSignal,
      ),
    ).toBe(true);
    expect(
      store
        .listProviders()
        .find((provider) => provider.id === "volcengine-agent-plan")
        ?.model_capabilities?.["glm-5.2"]?.reasoning_effort,
    ).toBeUndefined();
    await expect(
      store.reasoningCapabilityForProfile(profile.id),
    ).resolves.toEqual({
      levels: ["high", "max"],
      default_level: "high",
    });
    await expect(store.listProfiles()).resolves.toEqual([
      expect.objectContaining({
        id: profile.id,
        effective_capability: expect.objectContaining({
          reasoning_effort: { levels: ["high", "max"], default_level: "high" },
        }),
      }),
    ]);
  });

  it("does not persist a partial Agent Plan effort matrix when any level fails", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, status: 200 } as Response)
      .mockResolvedValueOnce({ ok: false, status: 422 } as Response);
    vi.stubGlobal("fetch", fetchMock);
    const store = new ModelProfileStore(configDir);

    await expect(
      store.testAgentPlanReasoningCapability({
        providerId: "volcengine-agent-plan",
        modelId: "qwen3.8-max-preview",
        apiKey: "test-key",
      }),
    ).resolves.toMatchObject({ ok: false });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(
      fetchMock.mock.calls.map(
        ([, init]) =>
          JSON.parse((init as RequestInit).body as string).reasoning_effort,
      ),
    ).toEqual(["low", "medium"]);
    expect(
      store
        .listProviders()
        .find((provider) => provider.id === "volcengine-agent-plan")
        ?.model_capabilities?.["qwen3.8-max-preview"]?.reasoning_effort,
    ).toBeUndefined();
    await expect(
      readFile(join(configDir, "cache", "model-capabilities.json"), "utf8"),
    ).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("applies one total timeout to the complete Agent Plan effort probe", async () => {
    vi.useFakeTimers();
    try {
      vi.stubGlobal(
        "fetch",
        vi.fn(
          (_url: string, init?: RequestInit) =>
            new Promise<Response>((_resolve, reject) => {
              init?.signal?.addEventListener("abort", () => {
                reject(
                  Object.assign(new Error("aborted"), { name: "AbortError" }),
                );
              });
            }),
        ),
      );
      const store = new ModelProfileStore(configDir);
      const resultPromise = store.testAgentPlanReasoningCapability({
        providerId: "volcengine-agent-plan",
        modelId: "doubao-seed-2.1-pro",
        apiKey: "test-key",
      });

      await vi.advanceTimersByTimeAsync(8_000);
      await expect(resultPromise).resolves.toEqual({
        ok: false,
        message: "Agent Plan 推理强度验证超时：完整矩阵继续隐藏。",
      });
      await expect(
        readFile(join(configDir, "cache", "model-capabilities.json"), "utf8"),
      ).rejects.toMatchObject({ code: "ENOENT" });
    } finally {
      vi.useRealTimers();
    }
  });

  it("keeps ark-code-latest effort hidden", async () => {
    const store = new ModelProfileStore(configDir);
    await expect(
      store.testAgentPlanReasoningCapability({
        providerId: "volcengine-agent-plan",
        modelId: "ark-code-latest",
        apiKey: "test-key",
      }),
    ).resolves.toMatchObject({ ok: false });
    expect(
      store
        .listProviders()
        .find((provider) => provider.id === "volcengine-agent-plan")
        ?.model_capabilities?.["ark-code-latest"]?.reasoning_effort,
    ).toBeUndefined();
  });

  it("exposes all current Kimi Coding Plan model IDs", () => {
    const store = new ModelProfileStore(configDir);
    const kimi = store
      .listProviders()
      .find((provider) => provider.id === "kimi-coding-plan");

    expect(kimi).toMatchObject({
      base_url: "https://api.kimi.com/coding/v1",
      default_model: "k3",
      models: ["k3", "k3-256k", "kimi-for-coding", "kimi-for-coding-highspeed"],
      docs_url: "https://www.kimi.com/code/docs/kimi-code/models.html",
    });
  });

  it("exposes Volcengine Coding Plan models as suggestions while allowing custom IDs", () => {
    const store = new ModelProfileStore(configDir);
    const volcengine = store
      .listProviders()
      .find((provider) => provider.id === "volcengine");

    expect(volcengine).toMatchObject({
      display_name: "火山 Coding Plan",
      base_url: "https://ark.cn-beijing.volces.com/api/coding/v3",
      allow_custom_model: true,
    });
    expect(volcengine?.models).toEqual(
      expect.arrayContaining([
        "deepseek-v4-pro",
        "glm-5.2",
        "kimi-k2.7-code",
        "minimax-m3",
      ]),
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

  it("does not abort a slow provider connection probe at the old eight-second boundary", async () => {
    vi.useFakeTimers();
    try {
      let signal: AbortSignal | undefined;
      vi.stubGlobal(
        "fetch",
        vi.fn((_input: string | URL | Request, init?: RequestInit) => {
          signal = init?.signal ?? undefined;
          return new Promise<Response>((_resolve, reject) => {
            signal?.addEventListener("abort", () => {
              reject(Object.assign(new Error("aborted"), { name: "AbortError" }));
            });
          });
        }),
      );

      const store = new ModelProfileStore(configDir);
      vi.spyOn(
        store as unknown as {
          appendConnectionUsage: (...args: unknown[]) => Promise<void>;
        },
        "appendConnectionUsage",
      ).mockResolvedValue(undefined);
      const result = store.testProfile({
        providerId: "qwen",
        displayName: "Qwen",
        modelId: "qwen3.7-max",
        baseURL: "https://dashscope.aliyuncs.com/apps/anthropic",
        protocol: "anthropic_messages",
        authScheme: "x-api-key",
        apiKey: "test-key",
      });

      await Promise.resolve();
      await Promise.resolve();
      expect(signal).toBeDefined();
      await vi.advanceTimersByTimeAsync(8_000);
      expect(signal?.aborted).toBe(false);
      await vi.advanceTimersByTimeAsync(22_000);
      await expect(result).resolves.toEqual({
        ok: false,
        message: "连接测试超时：30 秒内未收到服务响应。",
      });
    } finally {
      vi.useRealTimers();
    }
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

  it("does not treat a profileless connection-test body as scoped capability metadata", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          ({
            ok: true,
            status: 200,
            json: async () => ({
              data: [{ id: "deepseek-v4-pro", context_window: 256_000 }],
            }),
          }) as Response,
      ),
    );
    const store = new ModelProfileStore(configDir);

    await expect(
      store.testProfile({
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "test-key",
        enabled: true,
        isDefault: true,
      }),
    ).resolves.toMatchObject({ ok: true });

    await expect(
      readFile(join(configDir, "cache", "model-capabilities.json"), "utf8"),
    ).rejects.toMatchObject({ code: "ENOENT" });
  });

  it("normalizes the Kimi Code base URL before testing", async () => {
    const fetchMock = vi.fn(
      async () => ({ ok: true, status: 200 }) as Response,
    );
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

  it("tests a direct Doubao Ark Model ID without requiring an endpoint", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ choices: [] }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await new ModelProfileStore(configDir).testProfile({
      providerId: "doubao",
      displayName: "Doubao Ark",
      modelId: "doubao-seed-2-1-pro-260628",
      baseURL: "https://ark.cn-beijing.volces.com/api/v3",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "test-key",
    });

    expect(result).toEqual({
      ok: true,
      message: "连接测试通过：服务已响应 chat/completions。",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          model: "doubao-seed-2-1-pro-260628",
          messages: [{ role: "user", content: "ping" }],
          stream: false,
        }),
      }),
    );
  });

  it("tests a Doubao Ark Endpoint ID on the official Responses endpoint", async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ output_text: "ok" }), { status: 200 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const result = await new ModelProfileStore(configDir).testProfile({
      providerId: "doubao",
      displayName: "Doubao Ark",
      modelId: "doubao-seed-2-1-pro-260628",
      baseURL: "https://ark.cn-beijing.volces.com/api/v3",
      protocol: "openai_responses",
      authScheme: "bearer",
      apiKey: "test-key",
      advanced: { endpointId: "ep-test" },
    });

    expect(result).toEqual({
      ok: true,
      message: "连接测试通过：服务已响应 responses。",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "https://ark.cn-beijing.volces.com/api/v3/responses",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          model: "ep-test",
          input: "ping",
          stream: false,
          store: false,
        }),
      }),
    );
  });

  it("saves direct Doubao Model IDs and keeps ep-* as a separate selector kind", async () => {
    const store = new ModelProfileStore(configDir);
    const direct = await store.saveProfile({
      providerId: "doubao",
      modelId: "doubao-seed-2-1-pro-260628",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "direct-key",
      isDefault: true,
    });
    const endpoint = await store.saveProfile({
      providerId: "doubao",
      modelId: "ep-custom-endpoint",
      protocol: "openai_responses",
      authScheme: "bearer",
      apiKey: "endpoint-key",
      advanced: { endpointId: "ep-custom-endpoint" },
      isDefault: false,
    });

    expect(direct).toMatchObject({
      model_id: "doubao-seed-2-1-pro-260628",
      model_display_name: "Doubao-Seed-2.1-Pro",
    });

    await expect(store.resolveRunProfile(direct.id)).resolves.toMatchObject({
      modelId: "doubao-seed-2-1-pro-260628",
      capabilityModelId: "doubao-seed-2-1-pro-260628",
      modelSelectorKind: "model",
      endpointSelector: undefined,
    });
    await expect(store.resolveRunProfile(endpoint.id)).resolves.toMatchObject({
      modelId: "ep-custom-endpoint",
      capabilityModelId: "ep-custom-endpoint",
      modelSelectorKind: "endpoint",
      endpointSelector: "ep-custom-endpoint",
    });
  });

  it("migrates the old Doubao display alias to the exact Model ID", async () => {
    const store = new ModelProfileStore(configDir);
    const saved = await store.saveProfile({
      providerId: "doubao",
      modelId: "doubao-seed-2.1-pro",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "legacy-key",
      isDefault: true,
    });

    expect(saved).toMatchObject({
      model_id: "doubao-seed-2-1-pro-260628",
      model_display_name: "Doubao-Seed-2.1-Pro",
    });
    await expect(store.resolveRunProfile(saved.id)).resolves.toMatchObject({
      modelId: "doubao-seed-2-1-pro-260628",
      capabilityModelId: "doubao-seed-2-1-pro-260628",
      modelSelectorKind: "model",
    });
  });

  it("fails closed for unsupported official protocols and keeps custom providers protocol-flexible", async () => {
    const store = new ModelProfileStore(configDir);

    await expect(
      store.saveProfile({
        providerId: "openai",
        displayName: "OpenAI",
        modelId: "gpt-5.1",
        baseURL: "https://api.openai.com/v1",
        protocol: "anthropic_messages",
        authScheme: "bearer",
        apiKey: "openai-key",
      }),
    ).rejects.toThrow("OpenAI 不支持协议 anthropic_messages");

    await expect(
      store.saveProfile({
        providerId: "anthropic",
        displayName: "Anthropic",
        modelId: "claude-sonnet-4-6",
        baseURL: "https://api.anthropic.com",
        protocol: "openai_chat",
        authScheme: "x-api-key",
        apiKey: "anthropic-key",
      }),
    ).rejects.toThrow("Anthropic 不支持协议 openai_chat");

    await expect(
      store.saveProfile({
        providerId: "anthropic",
        displayName: "Anthropic",
        modelId: "claude-sonnet-4-6",
        baseURL: "https://api.anthropic.com",
        protocol: "openai_responses",
        authScheme: "x-api-key",
        apiKey: "anthropic-key",
      }),
    ).rejects.toThrow("Anthropic 不支持协议 openai_responses");

    for (const protocol of THREE_LLM_PROTOCOLS) {
      await expect(
        store.saveProfile({
          providerId: "custom",
          displayName: "Custom API",
          modelId: `custom-${protocol}`,
          baseURL: "https://gateway.test/v1",
          protocol,
          authScheme: "none",
          isDefault: false,
        }),
      ).resolves.toMatchObject({
        protocol,
        base_url: "https://gateway.test/v1",
      });
    }
  });

  it("tests Chat, Responses, and Messages with protocol-correct paths and bodies", async () => {
    const requests: Array<{ url: string; headers: Headers; body: Record<string, unknown> }> = [];
    vi.stubGlobal("fetch", vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      requests.push({
        url: String(input),
        headers: new Headers(init?.headers),
        body: JSON.parse(String(init?.body)) as Record<string, unknown>,
      });
      return new Response(JSON.stringify({ content: [{ type: "text", text: "ok" }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }));
    const store = new ModelProfileStore(configDir);

    await store.testProfile({
      providerId: "custom", modelId: "same-model", baseURL: "https://gateway.test/v1",
      protocol: "openai_chat", authScheme: "bearer", apiKey: "chat-key",
    });
    await store.testProfile({
      providerId: "custom", modelId: "same-model", baseURL: "https://gateway.test/v1",
      protocol: "openai_responses", authScheme: "api-key", apiKey: "responses-key",
    });
    await store.testProfile({
      providerId: "custom", modelId: "same-model", baseURL: "https://gateway.test",
      protocol: "anthropic_messages", authScheme: "x-api-key", apiKey: "messages-key",
    });

    expect(requests.map((request) => request.url)).toEqual([
      "https://gateway.test/v1/chat/completions",
      "https://gateway.test/v1/responses",
      "https://gateway.test/v1/messages",
    ]);
    expect(requests[0].body).toMatchObject({ messages: expect.any(Array), stream: false });
    expect(requests[0].body).not.toHaveProperty("max_tokens");
    expect(requests[1].body).toMatchObject({ input: "ping", store: false, stream: false });
    expect(requests[1].body).not.toHaveProperty("max_output_tokens");
    expect(requests[1].headers.get("api-key")).toBe("responses-key");
    expect(requests[2].body).toMatchObject({ messages: expect.any(Array), max_tokens: 64 });
    expect(requests[2].headers.get("x-api-key")).toBe("messages-key");
    expect(requests[2].headers.get("anthropic-version")).toBe("2023-06-01");
  });

  it("surfaces bounded provider diagnostics for failed protocol probes", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      error: {
        code: "invalid_parameter",
        message: "max_output_tokens must be greater than the reasoning budget",
      },
    }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    })));
    const store = new ModelProfileStore(configDir);

    await expect(store.testProfile({
      providerId: "custom",
      modelId: "reasoning-model",
      baseURL: "https://gateway.test/v1",
      protocol: "openai_responses",
      authScheme: "bearer",
      apiKey: "secret-key",
    })).resolves.toMatchObject({
      ok: false,
      message: expect.stringContaining("invalid_parameter: max_output_tokens must be greater than the reasoning budget"),
    });
  });

  it("merges refreshed provider models, fails closed without a key, and marks catalog_strategy none unavailable", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request) => {
      const url = String(input);
      if (url.endsWith("/models")) {
        return new Response(
          JSON.stringify({
            data: [
              { id: "gpt-5.1", context_window: 128000 },
              { id: "gpt-5.2", context_window: 256000 },
            ],
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }
      throw new Error(`Unexpected request ${url}`);
    });
    vi.stubGlobal("fetch", fetchMock);
    const store = new ModelProfileStore(configDir);

    await expect(
      store.refreshProviderCatalog({
        providerId: "openai",
        displayName: "OpenAI",
        modelId: "gpt-5.1",
        baseURL: "https://api.openai.com/v1",
        protocol: "openai_chat",
        authScheme: "bearer",
      }),
    ).rejects.toThrow("请先填写 API key，再刷新模型目录。");
    expect(fetchMock).not.toHaveBeenCalled();

    await expect(
      store.refreshProviderCatalog({
        providerId: "openai",
        displayName: "OpenAI",
        modelId: "gpt-5.1",
        baseURL: "https://api.openai.com/v1",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "openai-key",
      }),
    ).resolves.toMatchObject({
      message: "已从 OpenAI API 同步 2 个模型。",
      provider: expect.objectContaining({
        catalog_source: "provider_api",
        models: expect.arrayContaining(["gpt-5.1", "gpt-5.2"]),
        model_capabilities: expect.objectContaining({
          "gpt-5.2": expect.objectContaining({ context_window: 256000 }),
        }),
      }),
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "https://api.openai.com/v1/models",
      expect.objectContaining({
        method: "GET",
        headers: { Authorization: "Bearer openai-key" },
      }),
    );

    await expect(
      store.refreshProviderCatalog({
        providerId: "doubao",
        displayName: "Doubao Ark",
        modelId: "doubao-seed-2-1-pro-260628",
        baseURL: "https://ark.cn-beijing.volces.com/api/v3",
        protocol: "openai_chat",
        authScheme: "bearer",
      }),
    ).resolves.toMatchObject({
      message:
        "Doubao Ark 未提供可由当前推理凭据调用的模型目录 API，请使用官方清单或手填模型 ID。",
      provider: expect.objectContaining({
        catalog_source: "unavailable",
      }),
    });
  });

  it("refreshes Qwen Token Plan models with its dedicated key and endpoint", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      data: [{ id: "qwen3.8-max-preview" }, { id: "qwen3.7-plus" }],
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await new ModelProfileStore(configDir).refreshProviderCatalog({
      providerId: "qwen-token-plan",
      modelId: "qwen3.8-max-preview",
      protocol: "openai_responses",
      authScheme: "bearer",
      apiKey: "token-plan-key",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/models",
      expect.objectContaining({
        method: "GET",
        headers: { Authorization: "Bearer token-plan-key" },
      }),
    );
    expect(result.provider.models).toEqual(expect.arrayContaining([
      "qwen3.7-plus",
      "qwen3.8-max-preview",
    ]));
  });

  it("honors a complete inference endpoint override without exposing arbitrary headers", async () => {
    const fetchMock = vi.fn(async () => new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const store = new ModelProfileStore(configDir);

    await store.testProfile({
      providerId: "custom",
      modelId: "custom-model",
      baseURL: "https://unused.test/v1",
      protocol: "openai_responses",
      authScheme: "none",
      advanced: { inferenceEndpoint: "https://inference.test/custom/run" },
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "https://inference.test/custom/run",
      expect.objectContaining({
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  it("writes connection-test request and nullable usage to an independent profile JSONL", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          ({
            ok: true,
            status: 200,
            headers: {
              get: (name: string) => {
                if (name === "x-request-id") return "provider-req-1";
                if (name === "x-ratelimit-remaining-requests") return "99";
                if (name === "x-ratelimit-remaining-tokens") return "12345";
                return null;
              },
            },
            json: async () => ({
              usage: {
                prompt_tokens: 20,
                cached_tokens: 5,
                completion_tokens: 1,
                total_tokens: 21,
              },
            }),
          }) as Response,
      ),
    );
    const store = new ModelProfileStore(configDir);
    await expect(
      store.testProfile({
        id: "profile-usage",
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "secret-key-not-for-ledger",
        enabled: true,
        isDefault: true,
      }),
    ).resolves.toMatchObject({ ok: true });

    const usageRoot = join(configDir, "usage", "model-profiles");
    expect(await readdir(usageRoot)).toEqual(["profile-usage.jsonl"]);
    const text = await readFile(join(usageRoot, "profile-usage.jsonl"), "utf8");
    const records = text
      .trim()
      .split("\n")
      .map((line) => JSON.parse(line));
    expect(records).toHaveLength(2);
    expect(records[0]).toMatchObject({
      kind: "model_request",
      purpose: "connection_test",
    });
    expect(records[0]).not.toHaveProperty("max_output_tokens");
    expect(records[1]).toMatchObject({
      kind: "model_usage",
      request_id: records[0].request_id,
      input_tokens: 20,
      cached_input_tokens: 5,
      uncached_input_tokens: 15,
      output_tokens: 1,
      provider_request_id: "provider-req-1",
      pricing_snapshot: { effective_date: "2026-07-19" },
      rate_limit_observation: expect.objectContaining({
        request_remaining: "99",
        token_remaining: "12345",
      }),
    });
    expect(text).not.toContain("secret-key-not-for-ledger");
    expect(
      (await stat(join(usageRoot, "profile-usage.jsonl"))).mode & 0o777,
    ).toBe(0o600);
  });

  it("tests the selected K3 model ID", async () => {
    const fetchMock = vi.fn(
      async () => ({ ok: true, status: 200 }) as Response,
    );
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
    const fetchMock = vi.fn(
      async () => ({ ok: true, status: 200 }) as Response,
    );
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
      profiles: Array<{
        keyLast4?: string;
        credential?: unknown;
        apiKey?: string;
        credentialRevision?: number;
      }>;
    };
    expect(persisted.profiles[0]).toMatchObject({
      keyLast4: "-key",
      credentialRevision: 1,
    });
    expect(persisted.profiles[0]?.credential).toBeUndefined();
    expect(persisted.profiles[0]?.apiKey).toBeUndefined();
    expect(rawModels).not.toContain("test-key");
    expect(await readFile(join(configDir, ".env"), "utf8")).toContain(
      `AWAKEN_API_KEY_${saved.id}="test-key"`,
    );
    if (process.platform !== "win32") {
      expect((await stat(join(configDir, ".env"))).mode & 0o777).toBe(0o600);
    }
  });

  it("stores organization management credentials separately and only returns a mask", async () => {
    const store = new ModelProfileStore(configDir);
    const profile = await store.saveProfile({
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_responses",
      apiKey: "inference-secret",
      managementApiKey: "organization-admin-secret",
      isDefault: true,
    });

    const rawModels = await readFile(modelsPath(), "utf8");
    const env = await readFile(join(configDir, ".env"), "utf8");
    expect(rawModels).not.toContain("organization-admin-secret");
    expect(env).toContain("organization-admin-secret");
    await expect(store.listProfiles()).resolves.toEqual([
      expect.objectContaining({
        has_management_key: true,
        masked_management_key: "•••• cret",
      }),
    ]);
    const rotated = await store.saveProfile({
      id: profile.id,
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_responses",
      managementApiKey: "rotated-organization-admin-secret",
      isDefault: true,
    });
    expect(rotated.credential_revision).toBe(profile.credential_revision);
  });

  it("increments credentialRevision only when an existing API key actually changes", async () => {
    const store = new ModelProfileStore(configDir);
    const created = await store.saveProfile({
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
    expect(created.credential_revision).toBe(1);
    await expect(store.resolveDefaultRunProfile()).resolves.toMatchObject({
      credentialRevision: 1,
    });

    const sameKey = await store.saveProfile({
      id: created.id,
      providerId: "deepseek",
      displayName: "DeepSeek same key",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "first-key",
      enabled: true,
      isDefault: true,
    });
    expect(sameKey.credential_revision).toBe(1);

    const metadataOnly = await store.saveProfile({
      id: created.id,
      providerId: "deepseek",
      displayName: "DeepSeek metadata only",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      enabled: true,
      isDefault: true,
    });
    expect(metadataOnly.credential_revision).toBe(1);

    const changedKey = await store.saveProfile({
      id: created.id,
      providerId: "deepseek",
      displayName: "DeepSeek rotated",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "second-key",
      enabled: true,
      isDefault: true,
    });
    expect(changedKey.credential_revision).toBe(2);

    const repeatedSecondKey = await store.saveProfile({
      id: created.id,
      providerId: "deepseek",
      displayName: "DeepSeek rotated",
      modelId: "deepseek-v4-pro",
      baseURL: "https://api.deepseek.com",
      protocol: "openai_chat",
      authScheme: "bearer",
      apiKey: "second-key",
      enabled: true,
      isDefault: true,
    });
    expect(repeatedSecondKey.credential_revision).toBe(2);
    await expect(store.resolveDefaultRunProfile()).resolves.toMatchObject({
      credentialRevision: 2,
    });
  });

  it("interprets legacy profiles without credentialRevision as stable revision 1", async () => {
    await writeModels(
      JSON.stringify({
        version: 1,
        profiles: [
          {
            id: "legacy-deepseek",
            providerId: "deepseek",
            displayName: "DeepSeek",
            modelId: "deepseek-v4-pro",
            baseURL: "https://api.deepseek.com",
            protocol: "openai_chat",
            authScheme: "bearer",
            enabled: true,
            isDefault: true,
            keyLast4: "-key",
          },
        ],
      }),
    );
    await new AwakenEnvStore(configDir).setKey("legacy-deepseek", "legacy-key");

    const store = new ModelProfileStore(configDir);
    await expect(store.listProfiles()).resolves.toEqual([
      expect.objectContaining({ credential_revision: 1 }),
    ]);
    await expect(store.resolveDefaultRunProfile()).resolves.toMatchObject({
      credentialRevision: 1,
    });
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
    const unreadableStore = new ModelProfileStore(
      configDir,
      unreadableKeyStore,
    );

    await expect(
      unreadableStore.resolveDefaultRunProfile(),
    ).rejects.toMatchObject({
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

  it("projects an Agent Plan profile as unreadable when account metadata is corrupt", async () => {
    await mkdir(join(configDir, "config"), { recursive: true });
    await writeFile(
      join(configDir, "config", "agent-plan-account.json"),
      "{not-json",
      "utf8",
    );
    const store = new ModelProfileStore(configDir);

    const saved = await store.saveProfile({
      providerId: "volcengine-agent-plan",
      modelId: "ark-code-latest",
      enabled: true,
      isDefault: true,
    });

    expect(saved).toMatchObject({
      has_key: false,
      profile_status: "unreadable",
    });
    await expect(store.resolveRunProfile(saved.id)).rejects.toMatchObject({
      name: "AwakenEnvStoreError",
      code: "unreadable",
    });
    await expect(store.listProfiles()).resolves.toEqual([
      expect.objectContaining({
        id: saved.id,
        profile_status: "unreadable",
      }),
    ]);
  });

  it("restores Agent Plan key, metadata, revision, and catalog when combined profile save fails", async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      const authorization =
        new Headers(init?.headers).get("Authorization") ?? "";
      if (authorization.includes("new-secret")) {
        await rm(modelsPath(), { force: true });
        await mkdir(modelsPath());
        return new Response(JSON.stringify({ data: [{ id: "glm-new" }] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ data: [{ id: "glm-old" }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const store = new ModelProfileStore(configDir);
    await store.saveAgentPlanAccount({ apiKey: "old-secret" });
    await writeModels(JSON.stringify({ version: 2, profiles: [] }));
    const metadataPath = join(configDir, "config", "agent-plan-account.json");
    const envPath = join(configDir, ".env");
    const previousMetadataRaw = await readFile(metadataPath, "utf8");
    const previousEnv = await readFile(envPath, "utf8");

    await expect(
      store.saveProfile({
        providerId: "volcengine-agent-plan",
        modelId: "glm-old",
        apiKey: "new-secret",
        enabled: true,
        isDefault: true,
      }),
    ).rejects.toBeInstanceOf(Error);

    expect(await readFile(metadataPath, "utf8")).toBe(previousMetadataRaw);
    expect(await readFile(envPath, "utf8")).toBe(previousEnv);
    await expect(store.getAgentPlanAccount()).resolves.toMatchObject({
      masked_key: "•••• cret",
      credential_revision: 1,
      model_ids: expect.arrayContaining([
        "ark-code-latest",
        "glm-old",
        "doubao-seedream-5.0-lite",
        "doubao-seedance-2.0",
      ]),
    });
    expect(
      store
        .listProviders()
        .find((provider) => provider.id === "volcengine-agent-plan")?.models,
    ).toEqual(["ark-code-latest"]);
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

  it("preserves custom Volcengine Coding Plan model names without inventing capabilities", async () => {
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
      maxTokens: undefined,
      contextWindow: undefined,
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
        baseURL: "https://api.deepseek.com/",
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

  it("allows the same OpenAI model to coexist as Chat and Responses profiles", async () => {
    const store = new ModelProfileStore(configDir);
    const responses = await store.saveProfile({
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_responses",
      apiKey: "openai-key",
      isDefault: true,
    });
    const chat = await store.saveProfile({
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_chat",
      apiKey: "openai-key",
      isDefault: false,
    });

    expect(chat.id).not.toBe(responses.id);
    await expect(store.listProfiles()).resolves.toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: responses.id, protocol: "openai_responses" }),
        expect.objectContaining({ id: chat.id, protocol: "openai_chat" }),
      ]),
    );
    await expect(store.resolveRunProfile(responses.id)).resolves.toMatchObject({
      protocol: "openai_responses",
      supportsNativePDF: true,
    });
  });

  it("routes the nine purpose/protocol combinations and inherits when no route is set", async () => {
    const store = new ModelProfileStore(configDir);
    const inherited = await store.saveProfile({
      providerId: "deepseek",
      modelId: "deepseek-v4-pro",
      protocol: "openai_chat",
      apiKey: "deepseek-key",
      isDefault: true,
    });
    const openaiChat = await store.saveProfile({
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_chat",
      apiKey: "openai-key",
      isDefault: false,
    });
    const openaiResponses = await store.saveProfile({
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_responses",
      apiKey: "openai-key",
      isDefault: false,
    });
    const anthropicMessages = await store.saveProfile({
      providerId: "anthropic",
      modelId: "claude-sonnet-4-6",
      protocol: "anthropic_messages",
      apiKey: "anthropic-key",
      isDefault: false,
    });

    const purposes: Array<{
      purpose: "thread_title" | "compact_summary" | "subagent";
      label: string;
    }> = [
      { purpose: "thread_title", label: "thread_title" },
      { purpose: "compact_summary", label: "compact_summary" },
      { purpose: "subagent", label: "subagent" },
    ];
    const routes = [
      { profile: openaiChat, protocol: "openai_chat" },
      { profile: openaiResponses, protocol: "openai_responses" },
      { profile: anthropicMessages, protocol: "anthropic_messages" },
    ] as const;

    for (const { purpose } of purposes) {
      for (const route of routes) {
        await expect(
          store.savePurposeRouting({ [purpose]: route.profile.id }),
        ).resolves.toEqual({ [purpose]: route.profile.id });
        await expect(
          store.resolvePurposeRunProfile(purpose, inherited.id),
        ).resolves.toMatchObject({
          profileId: route.profile.id,
          protocol: route.protocol,
        });
      }
    }

    await expect(store.savePurposeRouting({})).resolves.toEqual({});
    await expect(
      store.resolvePurposeRunProfile("scheduled_task", inherited.id),
    ).resolves.toMatchObject({
      profileId: inherited.id,
      protocol: "openai_chat",
    });
  });

  it("falls back to inherited profile when a routed purpose profile is disabled", async () => {
    const store = new ModelProfileStore(configDir);
    await store.saveProfile({
      providerId: "openai",
      modelId: "gpt-5.1",
      protocol: "openai_responses",
      apiKey: "openai-key",
      isDefault: true,
    });
    const inherited = await store.saveProfile({
      providerId: "deepseek",
      modelId: "deepseek-v4-pro",
      protocol: "openai_chat",
      apiKey: "deepseek-key",
      isDefault: false,
    });
    const disabledRoute = await store.saveProfile({
      providerId: "anthropic",
      modelId: "claude-sonnet-4-6",
      protocol: "anthropic_messages",
      apiKey: "anthropic-key",
      enabled: true,
      isDefault: false,
    });

    await store.savePurposeRouting({ scheduled_task: disabledRoute.id });
    await store.saveProfile({
      id: disabledRoute.id,
      providerId: "anthropic",
      modelId: "claude-sonnet-4-6",
      protocol: "anthropic_messages",
      apiKey: "anthropic-key",
      enabled: false,
      isDefault: false,
    });

    await expect(
      store.resolvePurposeRunProfile("scheduled_task", inherited.id),
    ).resolves.toMatchObject({
      profileId: inherited.id,
      protocol: inherited.protocol,
    });
  });

  it("migrates a v2 profile to v3 while preserving its explicit protocol", async () => {
    await writeModels(JSON.stringify({
      version: 2,
      profiles: [{
        id: "legacy-chat",
        providerId: "deepseek",
        displayName: "Legacy Chat",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        enabled: true,
        isDefault: true,
      }],
    }));
    const store = new ModelProfileStore(configDir);

    await expect(store.listProfiles()).resolves.toEqual([
      expect.objectContaining({ id: "legacy-chat", protocol: "openai_chat" }),
    ]);
    expect(JSON.parse(await readFile(modelsPath(), "utf8"))).toMatchObject({
      version: 3,
      profiles: [expect.objectContaining({ protocol: "openai_chat" })],
    });
  });

  it("migrates retired Anthropic chat profiles back onto anthropic_messages with the official base URL and auth scheme", async () => {
    await writeModels(JSON.stringify({
      version: 2,
      profiles: [{
        id: "legacy-anthropic",
        providerId: "anthropic",
        displayName: "Legacy Anthropic",
        modelId: "claude-sonnet-4-6",
        baseURL: "https://api.anthropic.com",
        protocol: "openai_chat",
        authScheme: "x-api-key",
        enabled: true,
        isDefault: true,
      }],
    }));
    const store = new ModelProfileStore(configDir);

    await expect(store.listProfiles()).resolves.toEqual([
      expect.objectContaining({
        id: "legacy-anthropic",
        protocol: "anthropic_messages",
        base_url: "https://api.anthropic.com",
        auth_scheme: "x-api-key",
      }),
    ]);
    expect(JSON.parse(await readFile(modelsPath(), "utf8"))).toMatchObject({
      version: 3,
      profiles: [expect.objectContaining({
        protocol: "anthropic_messages",
        baseURL: "https://api.anthropic.com",
      })],
    });
  });

  it("keeps Agent Plan messages profiles on the historical /api/plan base URL during migration", async () => {
    await writeModels(JSON.stringify({
      version: 2,
      profiles: [{
        id: "legacy-agent-plan",
        providerId: "volcengine-agent-plan",
        displayName: "Legacy Agent Plan",
        modelId: "ark-code-latest",
        baseURL: "https://ark.cn-beijing.volces.com/api/plan",
        protocol: "anthropic_messages",
        authScheme: "bearer",
        enabled: true,
        isDefault: true,
      }],
    }));
    const store = new ModelProfileStore(configDir);

    await expect(store.listProfiles()).resolves.toEqual([
      expect.objectContaining({
        id: "legacy-agent-plan",
        protocol: "anthropic_messages",
        base_url: "https://ark.cn-beijing.volces.com/api/plan",
        auth_scheme: "bearer",
      }),
    ]);
    expect(JSON.parse(await readFile(modelsPath(), "utf8"))).toMatchObject({
      version: 3,
      profiles: [expect.objectContaining({
        protocol: "anthropic_messages",
        baseURL: "https://ark.cn-beijing.volces.com/api/plan",
      })],
    });
  });

  it("rejects guessed protocols for custom providers", async () => {
    const store = new ModelProfileStore(configDir);

    await expect(
      store.saveProfile({
        providerId: "custom",
        displayName: "Custom API",
        modelId: "custom-model",
        baseURL: "https://gateway.test/v1",
        protocol: "guessed_protocol",
        authScheme: "none",
      }),
    ).rejects.toThrow("不支持的模型协议：guessed_protocol");
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
    expect(
      profiles.find((profile) => profile.id === addedProfile.id),
    ).toMatchObject({
      model_id: "deepseek-v4-flash",
      has_key: true,
      masked_key: "•••• -key",
      is_default: false,
    });
  });

  it("does not reuse a saved template key when the current test form is empty", async () => {
    const fetchMock = vi.fn(
      async () => ({ ok: true, status: 200 }) as Response,
    );
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
      ok: false,
      message: "连接测试失败：请先填写 API key。",
    });
    expect(fetchMock).not.toHaveBeenCalled();
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
            baseURL: "https://api.deepseek.com/",
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
      base_url: "https://api.deepseek.com",
      has_key: true,
      masked_key: "•••• new1",
    });
  });

  describe("fetchBalances", () => {
    it("explains Google Cloud monitoring setup without aggregating local usage", async () => {
      const store = new ModelProfileStore(configDir);
      const profile = await store.saveProfile({
        providerId: "google",
        modelId: "gemini-2.5-pro",
        apiKey: "google-key",
        isDefault: true,
      });
      await writeFile(join(configDir, "CURRENT"), "gen-001\n", "utf8");
      const sessionDir = join(configDir, "store-generations", "gen-001", "sessions", "thread-1");
      await mkdir(sessionDir, { recursive: true });
      await writeFile(join(sessionDir, "journal.jsonl"), `${JSON.stringify({
        schema_version: 2,
        kind: "model_usage",
        payload: {
          profile_id: profile.id,
          status: "succeeded",
          input_tokens: 100,
          cached_input_tokens: 40,
          output_tokens: 20,
          calculated_cost_usd: 0.01,
        },
      })}\n`, "utf8");

      const balances = await store.fetchBalances();
      expect(balances).toEqual([
        expect.objectContaining({
          provider_id: "google",
          status: "requires_setup",
          error: expect.stringContaining("Google Cloud"),
          setup_url: expect.stringContaining("gemini-api/docs/billing"),
        }),
      ]);
      expect(balances[0]).not.toHaveProperty("local_usage");
    });

    it("queries the official Kimi API balance endpoint", async () => {
      vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
        code: 0,
        data: {
          available_balance: 12.5,
          voucher_balance: 2.5,
          cash_balance: 10,
        },
        scode: "0x0",
        status: true,
      }), { status: 200, headers: { "Content-Type": "application/json" } })));
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
        status: "available",
        detail: {
          kind: "amount",
          amounts: [{ currency: "CNY", total: 12.5, granted: 2.5, topped_up: 10 }],
        },
      });
    });

    it("queries MiniMax Token Plan quota with the saved subscription key", async () => {
      const fetchMock = vi.fn(async (input: string | URL | Request) => {
        expect(String(input)).toBe("https://www.minimaxi.com/v1/token_plan/remains");
        return new Response(JSON.stringify({
          model_remains: [{
            model_name: "general",
            current_interval_remaining_percent: 75,
            current_weekly_remaining_percent: 60,
          }, {
            model_name: "video",
            current_interval_remaining_percent: 10,
          }],
        }), { status: 200, headers: { "Content-Type": "application/json" } });
      });
      vi.stubGlobal("fetch", fetchMock);
      const store = new ModelProfileStore(configDir);
      await store.saveProfile({
        providerId: "minimax-token-plan",
        modelId: "MiniMax-M3",
        apiKey: "minimax-plan-key",
        enabled: true,
        isDefault: true,
      });

      await expect(store.fetchBalances()).resolves.toEqual([
        expect.objectContaining({
          provider_id: "minimax-token-plan",
          status: "available",
          detail: {
            kind: "quota",
            quotas: [
              expect.objectContaining({
                name: "5 小时额度",
                remaining: 75,
                unit: "percent",
              }),
              expect.objectContaining({
                name: "周额度",
                remaining: 60,
                unit: "percent",
              }),
            ],
          },
        }),
      ]);
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it("explains management authorization for Qwen and Doubao instead of using inference keys", async () => {
      const fetchMock = vi.fn();
      vi.stubGlobal("fetch", fetchMock);
      const store = new ModelProfileStore(configDir);
      await store.saveProfile({
        providerId: "qwen",
        modelId: "qwen3.8-max",
        apiKey: "dashscope-key",
        enabled: true,
        isDefault: true,
      });
      await store.saveProfile({
        providerId: "doubao",
        modelId: "doubao-seed-2-1-pro-260628",
        apiKey: "ark-inference-key",
        enabled: true,
        isDefault: false,
      });

      const balances = await store.fetchBalances();

      expect(balances).toEqual([
        expect.objectContaining({
          provider_id: "qwen",
          status: "requires_setup",
          error: expect.stringContaining("AccessKey"),
          setup_url: expect.stringContaining("model-telemetry"),
        }),
        expect.objectContaining({
          provider_id: "doubao",
          status: "requires_setup",
          error: expect.stringContaining("AK/SK"),
          setup_url: expect.stringContaining("GetInferenceUsage"),
        }),
      ]);
      expect(fetchMock).not.toHaveBeenCalled();
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
