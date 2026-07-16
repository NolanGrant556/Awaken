import fs from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SearchSettingsStore } from "./search-settings-store.js";

const roots: string[] = [];

async function fixture() {
  const root = await fs.mkdtemp(path.join(tmpdir(), "awaken-search-"));
  roots.push(root);
  const config = path.join(root, ".awaken", "config");
  return { config, store: new SearchSettingsStore(config) };
}

afterEach(async () => {
  vi.unstubAllGlobals();
  await Promise.all(
    roots
      .splice(0)
      .map((root) => fs.rm(root, { recursive: true, force: true })),
  );
});

describe("SearchSettingsStore", () => {
  it("defaults to Exa enabled and Tavily disabled", async () => {
    const { store } = await fixture();
    await expect(store.snapshot()).resolves.toEqual({
      providers: {
        exa: { enabled: true },
        tavily: { enabled: false },
      },
    });
    await expect(store.runtimeEnvironment()).resolves.toEqual({
      AWAKEN_WEB_SEARCH_PROVIDERS: "exa",
    });
  });

  it("persists independent channel switches and injects all enabled providers", async () => {
    const { store, config } = await fixture();
    await store.save({ provider: "tavily", enabled: true });
    await expect(store.runtimeEnvironment()).resolves.toEqual({
      AWAKEN_WEB_SEARCH_PROVIDERS: "exa,tavily",
    });
    const raw = await fs.readFile(path.join(config, "search.json"), "utf8");
    expect(JSON.parse(raw)).toEqual({
      version: 1,
      providers: {
        exa: { enabled: true },
        tavily: { enabled: true },
      },
    });
  });

  it("supports disabling every provider", async () => {
    const { store } = await fixture();
    await store.save({ provider: "exa", enabled: false });
    await expect(store.runtimeEnvironment()).resolves.toEqual({
      AWAKEN_WEB_SEARCH_PROVIDERS: "",
    });
  });

  it("fails closed for corrupt metadata", async () => {
    const { store, config } = await fixture();
    await fs.mkdir(config, { recursive: true });
    await fs.writeFile(path.join(config, "search.json"), "{broken", "utf8");
    await expect(store.snapshot()).rejects.toThrow("无法读取");
    await expect(store.runtimeEnvironment()).rejects.toThrow("无法读取");
  });

  it("does not publish a failed metadata write", async () => {
    const { config } = await fixture();
    const store = new SearchSettingsStore(config, {
      writeSettingsFile: async () => {
        throw new Error("disk full");
      },
    });
    await expect(
      store.save({ provider: "tavily", enabled: true }),
    ).rejects.toThrow("disk full");
    await expect(store.snapshot()).resolves.toEqual({
      providers: {
        exa: { enabled: true },
        tavily: { enabled: false },
      },
    });
  });

  it("tests Exa and Tavily independently with their keyless protocols", async () => {
    const requests: Request[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        const request = new Request(input, init);
        requests.push(request);
        return request.url.includes("mcp.exa.ai")
          ? new Response(
              JSON.stringify({
                jsonrpc: "2.0",
                id: 1,
                result: {
                  content: [
                    {
                      type: "text",
                      text: "Title: Awaken\nURL: https://example.com",
                    },
                  ],
                },
              }),
              { status: 200 },
            )
          : new Response(
              JSON.stringify({ results: [], response_time: 0.4 }),
              { status: 200 },
            );
      }),
    );
    const { store } = await fixture();
    await expect(store.testProvider("exa")).resolves.toEqual({
      ok: true,
      message: "Exa 连接成功。",
    });
    await expect(store.testProvider("tavily")).resolves.toEqual({
      ok: true,
      message: "Tavily 连接成功。",
    });
    expect(requests[0]?.url).toContain("mcp.exa.ai");
    expect(requests[1]?.url).toBe("https://api.tavily.com/search");
    expect(requests[1]?.headers.get("X-Tavily-Access-Mode")).toBe("keyless");
  });

  it("rejects a successful HTTP response with an invalid provider schema", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 200 })),
    );
    const { store } = await fixture();
    await expect(store.testProvider("tavily")).resolves.toEqual({
      ok: false,
      message: "Tavily 返回的响应格式无效。",
    });
  });
});
