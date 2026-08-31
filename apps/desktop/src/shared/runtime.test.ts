import { describe, expect, it } from "vitest";
import {
  calculateAutoCompactThreshold,
  effectiveContextWindow,
  normalizeModelProfileBalance,
  normalizeModelProfileRunConfig,
  normalizeModelProfileSummary,
  normalizeModelProvider,
  normalizeRuntimeRun,
  normalizeRuntimeThread,
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

  it("treats the configured context window as the available input budget", () => {
    expect(effectiveContextWindow(1_000_000)).toBe(1_000_000);
    expect(calculateAutoCompactThreshold(1_000_000)).toBe(900_000);
    expect(calculateAutoCompactThreshold(8_000)).toBe(7_200);
    expect(effectiveContextWindow(undefined)).toBe(0);
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

  it("normalizes provider setup links and percentage quota units", () => {
    expect(normalizeModelProfileBalance({
      profile_id: "qwen-usage",
      provider_id: "qwen",
      model_id: "qwen3.8-max",
      status: "requires_setup",
      error: "需要独立管理授权",
      setup_url: "https://help.aliyun.com/zh/model-studio/model-telemetry",
      setup_label: "查看 Qwen 监控与授权",
      detail: {
        kind: "quota",
        quotas: [{
          name: "当前窗口",
          limit: 100,
          used: 25,
          remaining: 75,
          unit: "percent",
          unlimited: true,
        }],
      },
    })).toMatchObject({
      status: "requires_setup",
      setupURL: "https://help.aliyun.com/zh/model-studio/model-telemetry",
      setupLabel: "查看 Qwen 监控与授权",
      detail: {
        kind: "quota",
        quotas: [{ unit: "percent", unlimited: true }],
      },
    });
  });
});

describe("runtime shared start run normalization", () => {
  it("normalizes durable thread and run model selections", () => {
    expect(normalizeRuntimeThread({
      thread_id: "thread-model",
      label: "model thread",
      workspace: { workspace_root: "/tmp", writable_roots: ["/tmp"] },
      selected_model_profile_id: "profile-kimi",
      reasoning_effort_by_profile: { "profile-kimi": "max" },
      created_at: "2026-07-29T00:00:00Z",
      updated_at: "2026-07-29T00:00:00Z",
    })).toMatchObject({
      selectedModelProfileId: "profile-kimi",
      reasoningEffortByProfile: { "profile-kimi": "max" },
    });

    expect(normalizeRuntimeRun({
      run_id: "run-model",
      thread_id: "thread-model",
      input: "hello",
      status: "finished",
      cancel_requested: false,
      model_profile_id: "profile-kimi",
      reasoning_effort: "max",
      created_at: "2026-07-29T00:00:00Z",
      updated_at: "2026-07-29T00:00:00Z",
    })).toMatchObject({
      modelProfileId: "profile-kimi",
      reasoningEffort: "max",
    });
  });

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
      modelProfileId: "profile-kimi",
      reasoningEffort: "max",
    });
    expect(wire.composer_tokens?.[0]?.leadingText).toBe(
      "现在我需要你帮我分析现有的财务数据风险，然后使用 ",
    );
    expect(wire).toMatchObject({
      model_profile_id: "profile-kimi",
      reasoning_effort: "max",
    });
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

import {
  normalizeScheduledTask,
  normalizeScheduledTaskEvent,
  normalizeScheduledTaskExecution,
  normalizeScheduledTaskSchedule,
  toScheduledTaskCreateWire,
  toScheduledTaskPatchWire,
  toScheduledTaskScheduleWire,
  type ScheduledTaskEventWire,
  type ScheduledTaskExecutionWire,
  type ScheduledTaskWire,
} from "./runtime.js";

describe("scheduled task wire normalization", () => {
  it("normalizes a cron schedule and round-trips it", () => {
    const schedule = normalizeScheduledTaskSchedule({
      kind: "cron",
      cron: "0 9 * * 1-5",
      timezone: "Asia/Shanghai",
    });
    expect(schedule).toEqual({
      kind: "cron",
      cron: "0 9 * * 1-5",
      timezone: "Asia/Shanghai",
    });
    expect(toScheduledTaskScheduleWire(schedule)).toEqual({
      kind: "cron",
      cron: "0 9 * * 1-5",
      timezone: "Asia/Shanghai",
    });
  });

  it("normalizes a one-time calendar schedule", () => {
    const schedule = normalizeScheduledTaskSchedule({
      kind: "calendar",
      run_at: "2026-08-01T09:00:00+08:00",
      timezone: "Asia/Shanghai",
    });
    expect(schedule).toEqual({
      kind: "calendar",
      runAt: "2026-08-01T09:00:00+08:00",
      timezone: "Asia/Shanghai",
      repeatEveryDays: undefined,
      repeatEnd: undefined,
    });
  });

  it("normalizes a repeating calendar schedule with until repeat end", () => {
    const schedule = normalizeScheduledTaskSchedule({
      kind: "calendar",
      run_at: "2026-08-01T09:00:00+08:00",
      timezone: "Asia/Shanghai",
      repeat_every_days: 3,
      repeat_end: { kind: "until", until: "2026-09-01T09:00:00+08:00" },
    });
    expect(schedule).toEqual({
      kind: "calendar",
      runAt: "2026-08-01T09:00:00+08:00",
      timezone: "Asia/Shanghai",
      repeatEveryDays: 3,
      repeatEnd: { kind: "until", until: "2026-09-01T09:00:00+08:00", count: undefined },
    });
    expect(toScheduledTaskScheduleWire(schedule)).toEqual({
      kind: "calendar",
      run_at: "2026-08-01T09:00:00+08:00",
      timezone: "Asia/Shanghai",
      repeat_every_days: 3,
      repeat_end: { kind: "until", until: "2026-09-01T09:00:00+08:00", count: undefined },
    });
  });

  it("normalizes count and never repeat ends", () => {
    const count = normalizeScheduledTaskSchedule({
      kind: "calendar",
      run_at: "2026-08-01T09:00:00Z",
      timezone: "UTC",
      repeat_every_days: 7,
      repeat_end: { kind: "count", count: 10 },
    });
    expect(count).toMatchObject({ repeatEnd: { kind: "count", count: 10 } });

    const never = normalizeScheduledTaskSchedule({
      kind: "calendar",
      run_at: "2026-08-01T09:00:00Z",
      timezone: "UTC",
      repeat_every_days: 1,
      repeat_end: { kind: "never" },
    });
    expect(never).toMatchObject({ repeatEnd: { kind: "never" } });
  });

  it("normalizes a full task with state, next run and last execution", () => {
    const wire: ScheduledTaskWire = {
      task_id: "sched_1",
      version: 3,
      name: "喝水提醒",
      enabled: true,
      project_id: "project-1",
      thread_id: "thread-9",
      schedule: { kind: "cron", cron: "0 * * * *", timezone: "UTC" },
      action: { kind: "reminder", content: "喝水" },
      execution: { profile_id: "", timeout_seconds: 600, misfire_grace_seconds: 300 },
      template_origin: { template_id: "hydration", source: "builtin" },
      state: "running",
      next_run_at: "2026-08-01T10:00:00Z",
      last_execution: {
        execution_id: "exec_1",
        status: "running",
        started_at: "2026-08-01T09:00:00Z",
      },
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-02T00:00:00Z",
    };
    expect(normalizeScheduledTask(wire)).toEqual({
      taskId: "sched_1",
      version: 3,
      name: "喝水提醒",
      enabled: true,
      projectId: "project-1",
      threadId: "thread-9",
      schedule: { kind: "cron", cron: "0 * * * *", timezone: "UTC" },
      action: { kind: "reminder", content: "喝水" },
      execution: { profileId: "", timeoutSeconds: 600, misfireGraceSeconds: 300 },
      templateOrigin: { templateId: "hydration", source: "builtin" },
      state: "running",
      invalidReason: undefined,
      nextRunAt: "2026-08-01T10:00:00Z",
      lastExecution: {
        executionId: "exec_1",
        status: "running",
        startedAt: "2026-08-01T09:00:00Z",
        finishedAt: undefined,
      },
      createdAt: "2026-07-01T00:00:00Z",
      updatedAt: "2026-07-02T00:00:00Z",
    });
  });

  it("keeps null projections null and drops empty optional fields", () => {
    const wire: ScheduledTaskWire = {
      task_id: "sched_2",
      version: 1,
      name: "暂停任务",
      enabled: false,
      project_id: "project-1",
      schedule: { kind: "cron", cron: "0 * * * *", timezone: "UTC" },
      action: { kind: "agent", content: "整理日报" },
      execution: { profile_id: "p1", timeout_seconds: 900, misfire_grace_seconds: 0 },
      state: "paused",
      next_run_at: null,
      last_execution: null,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
    };
    const task = normalizeScheduledTask(wire);
    expect(task.nextRunAt).toBeNull();
    expect(task.lastExecution).toBeNull();
    expect(task.threadId).toBeUndefined();
    expect(task.templateOrigin).toBeUndefined();
  });

  it("normalizes scheduled Agent composer tokens without flattening their identity", () => {
    const task = normalizeScheduledTask({
      task_id: "sched_skill",
      version: 1,
      name: "Skill 日报",
      enabled: true,
      project_id: "project-1",
      schedule: { kind: "cron", cron: "0 9 * * *", timezone: "UTC" },
      action: {
        kind: "agent",
        content: "生成日报",
        composer_tokens: [
          {
            id: "skill:daily-report:project",
            type: "skill",
            label: "daily-report",
            leadingText: "请使用 ",
            payload: {
              skillId: "daily-report",
              sourceOrigin: "project",
              skillFilePath: "/workspace/.awaken/skills/daily-report/SKILL.md",
              rootDir: "/workspace/.awaken/skills/daily-report",
            },
          },
        ],
      },
      execution: { profile_id: "", timeout_seconds: 600, misfire_grace_seconds: 600 },
      state: "enabled",
      next_run_at: null,
      last_execution: null,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
    });

    expect(task.action.composerTokens).toEqual([
      expect.objectContaining({
        id: "skill:daily-report:project",
        leadingText: "请使用 ",
        payload: expect.objectContaining({ skillId: "daily-report", sourceOrigin: "project" }),
      }),
    ]);
  });

  it("normalizes executions and treats the Go zero time as absent", () => {
    const queued: ScheduledTaskExecutionWire = {
      execution_id: "exec_1",
      task_id: "sched_1",
      task_version: 2,
      trigger: "manual",
      scheduled_for: "2026-08-01T09:00:00Z",
      started_at: "0001-01-01T00:00:00Z",
      status: "queued",
    };
    expect(normalizeScheduledTaskExecution(queued)).toEqual({
      executionId: "exec_1",
      taskId: "sched_1",
      taskVersion: 2,
      trigger: "manual",
      scheduledFor: "2026-08-01T09:00:00Z",
      startedAt: undefined,
      finishedAt: undefined,
      status: "queued",
      runId: undefined,
      threadId: undefined,
      errorCode: undefined,
      errorMessage: undefined,
    });

    const failed: ScheduledTaskExecutionWire = {
      execution_id: "exec_2",
      task_id: "sched_1",
      task_version: 2,
      trigger: "scheduled",
      scheduled_for: "2026-08-01T09:00:00Z",
      started_at: "2026-08-01T09:00:01Z",
      finished_at: "2026-08-01T09:10:00Z",
      status: "error",
      run_id: "run-1",
      thread_id: "thread-9",
      error_code: "EXECUTION_TIMEOUT",
      error_message: "execution timed out",
    };
    expect(normalizeScheduledTaskExecution(failed)).toMatchObject({
      status: "error",
      runId: "run-1",
      threadId: "thread-9",
      errorCode: "EXECUTION_TIMEOUT",
      errorMessage: "execution timed out",
      finishedAt: "2026-08-01T09:10:00Z",
    });
  });

  it("normalizes events and drops empty optional fields", () => {
    const notice: ScheduledTaskEventWire = {
      type: "notice",
      task_id: "sched_1",
      project_id: "project-1",
      execution_id: "exec_1",
      run_id: "run-1",
      thread_id: "thread-9",
      task_name: "喝水提醒",
      status: "success",
      content: "该喝水了",
    };
    expect(normalizeScheduledTaskEvent(notice)).toEqual({
      type: "notice",
      taskId: "sched_1",
      projectId: "project-1",
      executionId: "exec_1",
      runId: "run-1",
      threadId: "thread-9",
      taskName: "喝水提醒",
      status: "success",
      content: "该喝水了",
    });

    const changed: ScheduledTaskEventWire = {
      type: "task_changed",
      task_id: "sched_1",
      status: "enabled",
    };
    expect(normalizeScheduledTaskEvent(changed)).toEqual({
      type: "task_changed",
      taskId: "sched_1",
      executionId: undefined,
      threadId: undefined,
      taskName: undefined,
      status: "enabled",
      content: undefined,
    });
  });

  it("converts create input to snake_case wire", () => {
    expect(
      toScheduledTaskCreateWire({
        name: "每日站会",
        enabled: true,
        projectId: "project-1",
        schedule: {
          kind: "calendar",
          runAt: "2026-08-01T09:00:00+08:00",
          timezone: "Asia/Shanghai",
          repeatEveryDays: 1,
          repeatEnd: { kind: "count", count: 30 },
        },
        action: {
          kind: "agent",
          content: "写日报",
          composerTokens: [
            {
              id: "skill:daily-report:project",
              type: "skill",
              label: "daily-report",
              payload: { skillId: "daily-report", sourceOrigin: "project" },
            },
          ],
        },
        execution: { profileId: "profile-1", timeoutSeconds: 1200, misfireGraceSeconds: 600 },
        templateOrigin: { templateId: "daily-report", source: "builtin" },
      }),
    ).toEqual({
      name: "每日站会",
      enabled: true,
      project_id: "project-1",
      schedule: {
        kind: "calendar",
        run_at: "2026-08-01T09:00:00+08:00",
        timezone: "Asia/Shanghai",
        repeat_every_days: 1,
        repeat_end: { kind: "count", count: 30, until: undefined },
      },
      action: {
        kind: "agent",
        content: "写日报",
        composer_tokens: [
          {
            id: "skill:daily-report:project",
            type: "skill",
            label: "daily-report",
            payload: { skillId: "daily-report", sourceOrigin: "project" },
          },
        ],
      },
      execution: { profile_id: "profile-1", timeout_seconds: 1200, misfire_grace_seconds: 600 },
      template_origin: { template_id: "daily-report", source: "builtin" },
    });
  });

  it("converts patch input to snake_case wire with expected_version and only set fields", () => {
    expect(
      toScheduledTaskPatchWire({
        expectedVersion: 4,
        name: "改名",
        schedule: { kind: "cron", cron: "30 8 * * *", timezone: "UTC" },
        action: {
          kind: "agent",
          content: "复盘",
          composerTokens: [
            {
              id: "skill:weekly-review:user",
              type: "skill",
              label: "weekly-review",
              payload: { skillId: "weekly-review", sourceOrigin: "user" },
            },
          ],
        },
      }),
    ).toEqual({
      expected_version: 4,
      name: "改名",
      enabled: undefined,
      project_id: undefined,
      schedule: { kind: "cron", cron: "30 8 * * *", timezone: "UTC" },
      action: {
        kind: "agent",
        content: "复盘",
        composer_tokens: [
          {
            id: "skill:weekly-review:user",
            type: "skill",
            label: "weekly-review",
            payload: { skillId: "weekly-review", sourceOrigin: "user" },
          },
        ],
      },
      execution: undefined,
      template_origin: undefined,
    });
  });
});

describe("runtime thread archive normalization", () => {
  const baseWire = {
    thread_id: "thread-1",
    label: "归档会话",
    workspace: { workspace_root: "/tmp" },
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-02T00:00:00Z",
  };

  it("maps archived, archived_at and source from the wire", () => {
    expect(
      normalizeRuntimeThread({
        ...baseWire,
        archived: true,
        archived_at: "2026-08-20T10:20:30Z",
        source: "scheduled_task",
      }),
    ).toMatchObject({
      archived: true,
      archivedAt: "2026-08-20T10:20:30Z",
      source: "scheduled_task",
    });
  });

  it("defaults legacy threads without archive fields to unarchived", () => {
    expect(normalizeRuntimeThread(baseWire)).toMatchObject({
      archived: false,
      archivedAt: undefined,
      source: undefined,
    });
  });

  it("treats the Go zero time archived_at as absent", () => {
    expect(
      normalizeRuntimeThread({
        ...baseWire,
        archived: false,
        archived_at: "0001-01-01T00:00:00Z",
        source: "",
      }),
    ).toMatchObject({
      archived: false,
      archivedAt: undefined,
      source: undefined,
    });
  });
});
