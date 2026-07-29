import { mkdir, mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AgentPlanAccountStore,
  AgentPlanAccountStoreError,
} from "./agent-plan-account-store.js";
import { ModelProfileStore } from "./model-profile-store.js";

let root: string;

beforeEach(async () => {
  root = await mkdtemp(join(tmpdir(), "awaken-agent-plan-"));
  vi.stubGlobal(
    "fetch",
    vi.fn(async () =>
      new Response(JSON.stringify({ data: [{ id: "model-exact-1" }, { id: "model-exact-2" }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
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
      model_ids: ["ark-code-latest", "model-exact-1", "model-exact-2"],
    });

    const metadata = await readFile(join(root, "config", "agent-plan-account.json"), "utf8");
    expect(metadata).not.toContain("agent-secret");
    expect((await stat(join(root, "config", "agent-plan-account.json"))).mode & 0o777).toBe(0o600);
    expect((await stat(join(root, ".env"))).mode & 0o777).toBe(0o600);
    expect(await readFile(join(root, ".env"), "utf8")).toContain(
      "AWAKEN_AGENT_PLAN_API_KEY_volcengine-agent-plan-default",
    );

    const rotated = await store.saveAccount({ apiKey: "agent-secret-2222" });
    expect(rotated.credential_revision).toBe(2);
    expect(rotated.masked_key).toBe("•••• 2222");
  });

  it("keeps fallback catalog when /models refresh fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("no", { status: 503 })));
    const account = await new AgentPlanAccountStore(root).saveAccount({ apiKey: "secret" });
    expect(account.model_ids).toEqual(["ark-code-latest"]);
    expect(account.has_key).toBe(true);
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
    await expect(profiles.deleteAgentPlanAccount()).rejects.toThrow("仍被模型配置引用");
    await profiles.deleteProfile(profile.id);
    await expect(profiles.deleteAgentPlanAccount()).resolves.toBeUndefined();
  });
});
