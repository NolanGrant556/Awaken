import {
  mkdir,
  mkdtemp,
  readFile,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AgentPlanAccountStore,
  AgentPlanAccountStoreError,
} from "./agent-plan-account-store.js";
import { AgentPlanMediaToolsStore } from "./agent-plan-media-tools-store.js";
import { ModelProfileStore } from "./model-profile-store.js";

let root: string;

beforeEach(async () => {
  root = await mkdtemp(join(tmpdir(), "awaken-agent-plan-"));
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [
              { id: "glm-5.2" },
              { id: "doubao-seedream-5-0-test" },
              { id: "doubao-seedance-2-0-test" },
              { id: "future-model" },
            ],
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
    ),
  );
});

afterEach(async () => {
  await rm(root, { recursive: true, force: true });
  vi.unstubAllGlobals();
});

describe("AgentPlanAccountStore", () => {
  it("stores masked metadata separately from a 0600 env secret and rotates revision", async () => {
    const store = new AgentPlanAccountStore(root);
    const first = await store.saveAccount({ apiKey: "agent-secret-1111" });
    expect(first).toMatchObject({
      account_id: "volcengine-agent-plan-default",
      has_key: true,
      masked_key: "•••• 1111",
      credential_revision: 1,
      model_ids: expect.arrayContaining([
        "ark-code-latest",
        "glm-5.2",
        "doubao-seedream-5-0-test",
        "doubao-seedance-2-0-test",
        "future-model",
        "doubao-seedream-5.0-lite",
        "doubao-seedance-2.0",
      ]),
    });
    expect(first.catalog).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: "ark-code-latest", kind: "llm" }),
        { id: "glm-5.2", kind: "llm" },
        { id: "doubao-seedream-5-0-test", kind: "image" },
        { id: "doubao-seedance-2-0-test", kind: "video" },
        { id: "future-model", kind: "unknown" },
        expect.objectContaining({
          id: "doubao-seedream-5.0-lite",
          kind: "image",
        }),
        expect.objectContaining({
          id: "doubao-seedance-2.0",
          kind: "video",
        }),
      ]),
    );

    const metadata = await readFile(
      join(root, "config", "agent-plan-account.json"),
      "utf8",
    );
    expect(metadata).not.toContain("agent-secret");
    expect(
      (await stat(join(root, "config", "agent-plan-account.json"))).mode &
        0o777,
    ).toBe(0o600);
    expect((await stat(join(root, ".env"))).mode & 0o777).toBe(0o600);
    expect(await readFile(join(root, ".env"), "utf8")).toContain(
      "AWAKEN_AGENT_PLAN_API_KEY_volcengine-agent-plan-default",
    );

    const rotated = await store.saveAccount({ apiKey: "agent-secret-2222" });
    expect(rotated.credential_revision).toBe(2);
    expect(rotated.masked_key).toBe("•••• 2222");
  });

  it("keeps the built-in official catalog when /models refresh fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("no", { status: 503 })),
    );
    const account = await new AgentPlanAccountStore(root).saveAccount({
      apiKey: "secret",
    });
    expect(account.catalog).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: "ark-code-latest", kind: "llm" }),
        expect.objectContaining({
          id: "doubao-seedream-5.0-lite",
          kind: "image",
        }),
        expect.objectContaining({
          id: "doubao-seedance-2.0",
          kind: "video",
        }),
      ]),
    );
    expect(account.has_key).toBe(true);
  });

  it("treats Agent Plan's missing /models route as a built-in catalog refresh", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 404 })),
    );
    const store = new AgentPlanAccountStore(root);
    const account = await store.saveAccount({ apiKey: "secret" });
    expect(account.catalog_updated_at).toEqual(expect.any(String));
    expect(account.catalog).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "doubao-seedream-5.0-lite",
          kind: "image",
        }),
        expect.objectContaining({
          id: "doubao-seedance-2.0-fast",
          kind: "video",
        }),
      ]),
    );
  });

  it("persists built-in media entries when reading an older version 2 account file", async () => {
    await mkdir(join(root, "config"), { recursive: true });
    await writeFile(
      join(root, "config", "agent-plan-account.json"),
      JSON.stringify({
        version: 2,
        accountId: "volcengine-agent-plan-default",
        providerId: "volcengine-agent-plan",
        credentialRevision: 1,
        modelIds: ["ark-code-latest"],
        catalog: [{ id: "ark-code-latest", kind: "llm" }],
      }),
      "utf8",
    );

    const account = await new AgentPlanAccountStore(root).getAccount();
    expect(account.catalog).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "doubao-seedream-5.0-lite",
          kind: "image",
        }),
      ]),
    );
    const persisted = JSON.parse(
      await readFile(join(root, "config", "agent-plan-account.json"), "utf8"),
    ) as { builtinCatalogRevision?: number; catalog?: Array<{ id?: string }> };
    expect(persisted.builtinCatalogRevision).toBe(1);
    expect(persisted.catalog?.map((entry) => entry.id)).toContain(
      "doubao-seedream-5.0-lite",
    );
  });

  it("classifies unreadable account metadata as a typed credential failure", async () => {
    await mkdir(join(root, "config"), { recursive: true });
    await writeFile(
      join(root, "config", "agent-plan-account.json"),
      "{not-json",
      "utf8",
    );
    const store = new AgentPlanAccountStore(root);

    await expect(store.getAccount()).rejects.toEqual(
      expect.objectContaining<Partial<AgentPlanAccountStoreError>>({
        name: "AgentPlanAccountStoreError",
        code: "unreadable",
      }),
    );
  });

  it("blocks account deletion while a profile references it", async () => {
    const profiles = new ModelProfileStore(root);
    await profiles.initialize();
    const profile = await profiles.saveProfile({
      providerId: "volcengine-agent-plan",
      modelId: "ark-code-latest",
      apiKey: "agent-secret",
      enabled: true,
      isDefault: true,
    });
    await expect(profiles.deleteAgentPlanAccount()).rejects.toThrow(
      "仍被模型配置",
    );
    await profiles.deleteProfile(profile.id);
    await expect(profiles.deleteAgentPlanAccount()).resolves.toBeUndefined();
  });

  it("keeps image and video models out of the Agent Plan main-model provider", async () => {
    const profiles = new ModelProfileStore(root);
    await profiles.saveAgentPlanAccount({ apiKey: "agent-secret" });
    const provider = profiles
      .listProviders()
      .find((item) => item.id === "volcengine-agent-plan");
    expect(provider?.models).toEqual(["ark-code-latest"]);
    expect(provider?.allow_custom_model).toBe(false);
    await expect(
      profiles.saveProfile({
        providerId: "volcengine-agent-plan",
        modelId: "doubao-seedream-5-0-test",
        enabled: true,
      }),
    ).rejects.toThrow("只能选择目录中已分类的对话或推理模型");
  });

  it("stores one enabled media model per capability and rejects cross-kind choices", async () => {
    const accountStore = new AgentPlanAccountStore(root);
    await accountStore.saveAccount({ apiKey: "agent-secret" });
    const mediaStore = new AgentPlanMediaToolsStore(root, accountStore);

    const image = await mediaStore.save({
      capability: "media.image.generate",
      modelId: "doubao-seedream-5-0-test",
    });
    expect(image.configurations).toEqual([
      expect.objectContaining({
        capability: "media.image.generate",
        model_id: "doubao-seedream-5-0-test",
        display_state: "ready_unverified",
      }),
    ]);

    const video = await mediaStore.save({
      capability: "media.video.generate",
      modelId: "doubao-seedance-2-0-test",
    });
    expect(video.configurations).toHaveLength(2);
    expect(
      video.configurations.filter(
        (configuration) => configuration.capability === "media.image.generate",
      ),
    ).toHaveLength(1);
    await expect(
      mediaStore.save({
        capability: "media.image.generate",
        modelId: "glm-5.2",
      }),
    ).rejects.toThrow("不在当前 Agent Plan 图片模型目录中");

    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ data: [{ id: "glm-5.2" }] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    await accountStore.refreshModelCatalog();
    expect(
      (await mediaStore.snapshot()).configurations.find(
        (configuration) => configuration.capability === "media.image.generate",
      )?.display_state,
    ).toBe("unavailable");
    await expect(
      mediaStore.save({
        capability: "media.image.generate",
        modelId: "doubao-seedream-5-0-test",
        enabled: false,
      }),
    ).resolves.toEqual(
      expect.objectContaining({
        configurations: expect.arrayContaining([
          expect.objectContaining({
            capability: "media.image.generate",
            enabled: false,
            display_state: "disabled",
          }),
        ]),
      }),
    );

    const metadata = await readFile(
      join(root, "config", "agent-plan-media-tools.json"),
      "utf8",
    );
    expect(metadata).not.toContain("agent-secret");
    expect(
      (await stat(join(root, "config", "agent-plan-media-tools.json"))).mode &
        0o777,
    ).toBe(0o600);
    const profiles = new ModelProfileStore(root);
    await profiles.initialize();
    await expect(profiles.deleteAgentPlanAccount()).rejects.toThrow(
      "生成工具配置引用",
    );
  });
});
