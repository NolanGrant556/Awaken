import { describe, expect, it } from "vitest";
import {
  calculateAutoCompactThreshold,
  effectiveContextWindow,
  normalizeModelProfileRunConfig,
  normalizeModelProfileSummary,
  normalizeModelProvider,
  normalizeRuntimeRun,
  normalizeStartRunRequest,
} from "./runtime.js";

describe("runtime shared model provider normalization", () => {
  it("normalizes model capability metadata", () => {
    const provider = normalizeModelProvider({
      id: "deepseek",
      display_name: "DeepSeek",
      base_url: "https://api.deepseek.com",
      protocol: "openai_chat",
      auth_scheme: "bearer",
      key_env_name: "DEEPSEEK_API_KEY",
      default_model: "deepseek-v4-pro",
      models: ["deepseek-v4-pro"],
      docs_url: "https://api-docs.deepseek.com/",
      api_docs_url: "https://platform.deepseek.com/api_keys",
      allow_custom_model: true,
      model_capabilities: {
        "deepseek-v4-pro": {
          context_window: 1_000_000,
          max_output_tokens: 384_000,
          default_max_tokens: 8192,
        },
      },
    });

    expect(provider.modelCapabilities["deepseek-v4-pro"]).toEqual({
      contextWindow: 1_000_000,
      maxOutputTokens: 384_000,
      defaultMaxTokens: 8192,
      defaultTemperature: undefined,
    });
    expect(provider.allowCustomModel).toBe(true);
  });

  it("calculates the auto compact threshold from effective context window", () => {
    expect(effectiveContextWindow(1_000_000)).toBe(984_000);
    expect(calculateAutoCompactThreshold(1_000_000)).toBe(885_600);
    expect(calculateAutoCompactThreshold(8_000)).toBe(0);
    expect(calculateAutoCompactThreshold(12_000, 20_000)).toBe(0);
  });

  it("defaults legacy credential revisions to 1 and sends explicit run revisions", () => {
    expect(
      normalizeModelProfileSummary({
        id: "profile-1",
        provider_id: "deepseek",
        display_name: "DeepSeek",
        model_id: "deepseek-v4-pro",
        base_url: "https://api.deepseek.com",
        protocol: "openai_chat",
        auth_scheme: "bearer",
        has_key: true,
        enabled: true,
        is_default: true,
        profile_status: "ready",
      }).credentialRevision,
    ).toBe(1);

    expect(
      normalizeModelProfileRunConfig({
        profileId: "profile-1",
        providerId: "deepseek",
        displayName: "DeepSeek",
        modelId: "deepseek-v4-pro",
        baseURL: "https://api.deepseek.com",
        protocol: "openai_chat",
        authScheme: "bearer",
        apiKey: "secret",
        credentialRevision: 4,
      }),
    ).toMatchObject({ credential_revision: 4 });
  });
});

describe("runtime shared start run normalization", () => {
  it("normalizes composer tokens returned on runtime runs", () => {
    expect(
      normalizeRuntimeRun({
        run_id: "run-1",
        thread_id: "thread-1",
        input: "hello",
        composer_tokens: [
          {
            id: "skill:deep-research:user",
            type: "skill",
            label: "deep-research",
            payload: {
              skillId: "deep-research",
              sourceOrigin: "user",
              skillFilePath: "/home/.awaken/skills/deep-research/SKILL.md",
            },
          },
        ],
        status: "running",
        created_at: "2026-07-09T00:00:00Z",
        updated_at: "2026-07-09T00:00:00Z",
      }),
    ).toMatchObject({
      runId: "run-1",
      composerTokens: [
        {
          id: "skill:deep-research:user",
          type: "skill",
          label: "deep-research",
          payload: {
            skillId: "deep-research",
            sourceOrigin: "user",
            skillFilePath: "/home/.awaken/skills/deep-research/SKILL.md",
          },
        },
      ],
    });
  });

  it("keeps token leadingText when normalizing runtime runs", () => {
    const normalized = normalizeRuntimeRun({
      run_id: "run-2",
      thread_id: "thread-2",
      input: "现在我需要你帮我分析现有的财务数据风险，然后使用 输出一份风险报告给我",
      composer_tokens: [
        {
          id: "skill:pptx:user",
          type: "skill",
          label: "pptx",
          leadingText: "现在我需要你帮我分析现有的财务数据风险，然后使用 ",
          payload: { skillId: "pptx", sourceOrigin: "user" },
        },
      ],
      status: "finished",
      created_at: "2026-07-13T00:00:00Z",
      updated_at: "2026-07-13T00:00:00Z",
    });
    expect(normalized.composerTokens?.[0]?.leadingText).toBe(
      "现在我需要你帮我分析现有的财务数据风险，然后使用 ",
    );
  });

  it("keeps token leadingText on the start run wire request", () => {
    const wire = normalizeStartRunRequest({
      input: "现在我需要你帮我分析现有的财务数据风险，然后使用 输出一份风险报告给我",
      composerTokens: [
        {
          id: "skill:pptx:user",
          type: "skill",
          label: "pptx",
          leadingText: "现在我需要你帮我分析现有的财务数据风险，然后使用 ",
          payload: { skillId: "pptx", sourceOrigin: "user" },
        },
      ],
      workspace: {
        workspaceRoot: "/workspace",
        readableRoots: ["/workspace"],
        writableRoots: ["/workspace"],
        outputRoots: ["/workspace"],
      },
      client: {
        sessionId: "session-2",
        requestId: "request-2",
        entrypoint: "desktop",
      },
    });
    expect(wire.composer_tokens?.[0]?.leadingText).toBe(
      "现在我需要你帮我分析现有的财务数据风险，然后使用 ",
    );
  });

  it("sends the project name so the agent can name where it is working", () => {
    const wire = normalizeStartRunRequest({
      input: "本项目有什么文件",
      workspace: {
        projectId: "project_1",
        projectName: "测试项目",
        workspaceRoot: "/workspace/01",
        readableRoots: ["/workspace/01", "/workspace/02"],
      },
      client: {
        sessionId: "session-3",
        requestId: "request-3",
        entrypoint: "desktop",
      },
    });
    expect(wire.workspace.project_name).toBe("测试项目");
    expect(wire.workspace.project_id).toBe("project_1");
  });

  it("passes composer tokens through the wire request without mutating input text", () => {
    expect(
      normalizeStartRunRequest({
        input: "summarize this folder",
        composerTokens: [
          {
            id: "skill:daily-followup:project",
            type: "skill",
            label: "Daily Followup",
            payload: {
              skillId: "daily-followup",
              sourceOrigin: "project",
              skillFilePath:
                "/workspace/.awaken/skills/daily-followup/SKILL.md",
              rootDir: "/workspace/.awaken/skills/daily-followup",
            },
          },
        ],
        workspace: {
          workspaceRoot: "/workspace",
          readableRoots: ["/workspace"],
          writableRoots: ["/workspace"],
          outputRoots: ["/workspace"],
        },
        client: {
          sessionId: "session-1",
          requestId: "request-1",
          entrypoint: "desktop",
        },
      }),
    ).toMatchObject({
      input: "summarize this folder",
      composer_tokens: [
        {
          id: "skill:daily-followup:project",
          type: "skill",
          label: "Daily Followup",
          payload: {
            skillId: "daily-followup",
            sourceOrigin: "project",
            skillFilePath:
              "/workspace/.awaken/skills/daily-followup/SKILL.md",
            rootDir: "/workspace/.awaken/skills/daily-followup",
          },
        },
      ],
    });
  });
});
