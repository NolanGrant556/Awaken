import { mkdtemp, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import {
  loadCapabilityOverrides,
  mergeCapabilityOverrides,
  parseProviderCapabilities,
  saveCapabilityOverrides,
  type CapabilityCacheScope,
} from "./model-capability-sync.js";

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
