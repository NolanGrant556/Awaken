import { mkdtemp, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  fetchProviderCatalog,
  loadCapabilityOverrides,
  mergeCapabilityOverrides,
  parseProviderCapabilities,
  saveCapabilityOverrides,
  type CapabilityCacheScope,
} from "./model-capability-sync.js";

afterEach(() => {
  vi.unstubAllGlobals();
});

function scope(overrides: Partial<CapabilityCacheScope> = {}): CapabilityCacheScope {
  return {
    provider_id: "volcengine-agent-plan",
    normalized_base_url: "https://ark.cn-beijing.volces.com/api/coding/v3",
    protocol_family: "openai_chat",
    profile_id: "profile-1",
    credential_revision: 1,
    selector_kind: "model",
    endpoint_contract_id: "openai_chat_completions_v1",
    ...overrides,
  };
}

describe("model capability sync", () => {
  it("parses Kimi Code model metadata field names", () => {
    expect(
      parseProviderCapabilities({
        data: [
          {
            id: "k3",
            context_length: 1_048_576,
            supports_image_in: true,
            supports_video_in: true,
          },
        ],
      }),
    ).toEqual({
      k3: {
        context_window: 1_048_576,
        supports_vision: true,
        supports_video: true,
      },
    });
  });

  it("normalizes explicit per-model protocol metadata without guessing from the provider", () => {
    expect(
      parseProviderCapabilities({
        data: [{
          id: "example-model",
          capabilities: {
            supported_protocols: ["chat_completions", "responses", "anthropic_messages"],
          },
        }],
      }),
    ).toEqual({
      "example-model": {
        supported_protocols: ["openai_chat", "openai_responses", "anthropic_messages"],
      },
    });
  });

  it("paginates Anthropic models and preserves API capability provenance", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      const page2 = url.includes("after_id=claude-sonnet-4-6");
      expect(new Headers(init?.headers).get("x-api-key")).toBe("adminless-inference-key");
      expect(new Headers(init?.headers).get("anthropic-version")).toBe("2023-06-01");
      return new Response(JSON.stringify(page2 ? {
        data: [{ id: "claude-opus-4-6", max_input_tokens: 1_000_000, max_tokens: 128_000 }],
        has_more: false,
      } : {
        data: [{
          id: "claude-sonnet-4-6",
          max_input_tokens: 1_000_000,
          max_tokens: 64_000,
          capabilities: { image_input: true, pdf_input: true, tool_use: true, prompt_caching: true },
        }],
        has_more: true,
        last_id: "claude-sonnet-4-6",
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchProviderCatalog(
      "anthropic", "https://api.anthropic.com", "adminless-inference-key", "x-api-key",
    );

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.models).toEqual(["claude-opus-4-6", "claude-sonnet-4-6"]);
    expect(result.capabilities["claude-sonnet-4-6"]).toMatchObject({
      max_input_tokens: 1_000_000,
      max_output_tokens: 64_000,
      supports_vision: true,
      supports_native_pdf: true,
      supports_tools: "ready",
      supports_prompt_cache: true,
      field_sources: {
        maxInputTokens: { source: "provider_api", observed_at: expect.any(String) },
      },
    });
  });

  it("uses Google's native model catalog and keeps input/output limits distinct", async () => {
    const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      expect(url).toContain("https://generativelanguage.googleapis.com/v1beta/models");
      expect(new Headers(init?.headers).get("x-goog-api-key")).toBe("google-key");
      const page2 = url.includes("pageToken=next-page");
      return new Response(JSON.stringify(page2 ? {
        models: [{ name: "models/gemini-2.5-flash", inputTokenLimit: 1_048_576, outputTokenLimit: 65_536 }],
      } : {
        models: [{ name: "models/gemini-2.5-pro", inputTokenLimit: 1_048_576, outputTokenLimit: 65_536 }],
        nextPageToken: "next-page",
      }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    vi.stubGlobal("fetch", fetchMock);

    const result = await fetchProviderCatalog(
      "google", "https://generativelanguage.googleapis.com/v1beta/openai", "google-key", "bearer",
    );

    expect(result.models).toEqual(["gemini-2.5-flash", "gemini-2.5-pro"]);
    expect(result.capabilities["gemini-2.5-pro"]).toMatchObject({
      max_input_tokens: 1_048_576,
      max_output_tokens: 65_536,
    });
    expect(result.capabilities["gemini-2.5-pro"].context_window).toBeUndefined();
  });

  it("reads DashScope's nested output.models catalog shape", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({
      request_id: "request-1",
      output: {
        models: [
          {
            model: "qwen3.8-max",
            name: "Qwen 3.8 Max",
            capabilities: {
              context_window: 1_048_576,
              max_input_tokens: 983_616,
              max_output_tokens: 65_536,
              tool_use: true,
              reasoning_effort: {
                levels: ["low", "medium", "xhigh"],
                default: "xhigh",
              },
            },
          },
          { model_name: "qwen3.7-plus" },
          { model: "happyhorse-video-1", model_type: "video" },
        ],
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } })));

    const result = await fetchProviderCatalog(
      "openai",
      "https://dashscope.aliyuncs.com/compatible-mode/v1",
      "dashscope-key",
      "bearer",
      undefined,
      "https://dashscope.aliyuncs.com/api/v1/models",
    );

    expect(result.models).toEqual(["qwen3.7-plus", "qwen3.8-max"]);
    expect(result.entries).toContainEqual({ id: "happyhorse-video-1", kind: "video" });
    expect(result.capabilities["qwen3.8-max"]).toMatchObject({
      context_window: 1_048_576,
      max_input_tokens: 983_616,
      max_output_tokens: 65_536,
      supports_tools: "ready",
      reasoning_effort: {
        levels: ["low", "high", "max"],
        default_level: "max",
        display_labels: { low: "low", high: "medium", max: "xhigh" },
      },
    });
    expect(result.capabilities["happyhorse-video-1"]).toBeUndefined();
  });

  it("preserves controlled reasoning capability when /models refreshes metadata", () => {
    expect(
      mergeCapabilityOverrides(
        [{
          scope: scope(),
          models: {
            "glm-5.2": {
              reasoning_effort: {
                levels: ["high", "max"],
                default_level: "high",
              },
            },
          },
        }],
        [{
          scope: scope(),
          models: {
            "glm-5.2": { context_window: 262_144 },
          },
        }],
      ),
    ).toEqual([{
      scope: scope(),
      models: {
        "glm-5.2": {
          reasoning_effort: {
            levels: ["high", "max"],
            default_level: "high",
          },
          context_window: 262_144,
        },
      },
    }]);
  });

  it("serializes cache writes by path and merges stale concurrent snapshots", async () => {
    const root = await mkdtemp(join(tmpdir(), "awaken-capability-cache-"));
    try {
      await Promise.all([
        saveCapabilityOverrides(root, [{
          scope: scope(), models: { shared: { context_window: 262_144 } },
        }]),
        saveCapabilityOverrides(root, [{
          scope: scope(), models: { shared: { max_output_tokens: 16_384 } },
        }]),
        ...Array.from({ length: 12 }, (_, index) =>
          saveCapabilityOverrides(root, [{
            scope: scope({ profile_id: `profile-${index + 2}` }),
            models: {
              [`model-${index}`]: { context_window: 32_000 + index },
            },
          }]),
        ),
      ]);

      const stored = await loadCapabilityOverrides(root);
      expect(stored.find((entry) => entry.scope.profile_id === "profile-1")?.models.shared).toEqual({
        context_window: 262_144,
        max_output_tokens: 16_384,
      });
      expect(stored).toHaveLength(13);
      expect((await readdir(root)).filter((name) => name.endsWith(".tmp"))).toEqual([]);
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });

  it("keeps official and custom endpoint scopes isolated", () => {
    const official = scope();
    const custom = scope({ normalized_base_url: "https://proxy.example.test/v1" });
    const merged = mergeCapabilityOverrides(
      [{ scope: official, models: { model: { context_window: 128_000 } } }],
      [{ scope: custom, models: { model: { context_window: 32_000 } } }],
    );
    expect(merged).toHaveLength(2);
  });

  it("ignores the provider-wide v1 cache as cold", async () => {
    const root = await mkdtemp(join(tmpdir(), "awaken-capability-v1-"));
    try {
      await writeFile(
        join(root, "model-capabilities.json"),
        JSON.stringify({ version: 1, providers: { deepseek: { model: { context_window: 1 } } } }),
      );
      expect(await loadCapabilityOverrides(root)).toEqual([]);
    } finally {
      await rm(root, { recursive: true, force: true });
    }
  });
});
