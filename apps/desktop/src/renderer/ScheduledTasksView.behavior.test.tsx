// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  RuntimeModelProfileSummary,
  RuntimePlugin,
  RuntimeProject,
  RuntimeSkill,
  ScheduledTask,
  ScheduledTaskEvent,
} from "../shared/runtime";
import type { RuntimeClient } from "./runtime/client";
import { ScheduledTasksView } from "./ScheduledTasksView";

const project: RuntimeProject = {
  projectId: "project-1",
  workspaceRoot: "/workspace/alpha",
  folders: [],
  displayName: "Alpha",
  state: "active",
  createdAt: "2026-07-01T00:00:00Z",
  updatedAt: "2026-07-01T00:00:00Z",
  version: 1,
};

const readyProfile: RuntimeModelProfileSummary = {
  id: "profile-1",
  providerId: "openai",
  displayName: "GPT 默认",
  modelId: "gpt-5",
  baseURL: "https://api.example.com",
  protocol: "openai_chat",
  authScheme: "bearer",
  hasKey: true,
  enabled: true,
  isDefault: true,
  profileStatus: "ready",
};

const dailyReportSkill: RuntimeSkill = {
  id: "daily-report",
  catalogEntryId: "catalog-daily-report",
  name: "daily-report",
  displayName: "日报 Skill",
  description: "生成项目日报",
  state: "loaded",
  sourceOrigin: "project",
  rootDir: "/workspace/alpha/.awaken/skills/daily-report",
  skillFilePath: "/workspace/alpha/.awaken/skills/daily-report/SKILL.md",
  projectPath: "/workspace/alpha",
  context: "inline",
};

function cronTask(overrides: Partial<ScheduledTask> = {}): ScheduledTask {
  return {
    taskId: "task-1",
    version: 2,
    name: "工作日早报",
    enabled: true,
    projectId: "project-1",
    schedule: { kind: "cron", cron: "30 9 * * 1-5", timezone: "Asia/Shanghai" },
    action: { kind: "agent", content: "整理新闻" },
    execution: { profileId: "", timeoutSeconds: 600, misfireGraceSeconds: 600 },
    state: "enabled",
    createdAt: "2026-07-01T00:00:00Z",
    updatedAt: "2026-07-01T00:00:00Z",
    ...overrides,
  };
}

function createRuntimeClientMock(overrides: Partial<RuntimeClient> = {}): RuntimeClient {
  return {
    listScheduledTasks: vi.fn(async () => []),
    listProjects: vi.fn(async () => [project]),
    listModelProfiles: vi.fn(async () => [readyProfile]),
    listPlugins: vi.fn(async () => []),
    getSystemTimezone: vi.fn(async () => "Asia/Shanghai"),
    subscribeScheduledTaskEvents: vi.fn(() => () => {}),
    subscribeComposerCatalogEvents: vi.fn(() => () => {}),
    createScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    patchScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    pauseScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    resumeScheduledTask: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    runScheduledTaskNow: vi.fn(async () => {
      throw new Error("not implemented");
    }),
    cancelScheduledTaskExecution: vi.fn(async () => undefined),
    deleteScheduledTask: vi.fn(async () => undefined),
    listScheduledTaskExecutions: vi.fn(async () => []),
    ...overrides,
  } as unknown as RuntimeClient;
}

async function settle(): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

function buttonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.textContent?.trim() === text,
  );
}

function buttonByAriaLabel(label: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll<HTMLButtonElement>("button")).find(
    (button) => button.getAttribute("aria-label") === label,
  );
}

async function click(element: Element | null | undefined): Promise<void> {
  expect(element, "expected element to exist").toBeTruthy();
  await act(async () => {
    element?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
  });
  await settle();
}

async function setFieldValue(
  element: HTMLInputElement | HTMLTextAreaElement | null | undefined,
  value: string,
): Promise<void> {
  expect(element, "expected field to exist").toBeTruthy();
  const prototype =
    element instanceof HTMLTextAreaElement
      ? window.HTMLTextAreaElement.prototype
      : window.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(prototype, "value")?.set;
  await act(async () => {
    setter?.call(element, value);
    element?.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await settle();
}

async function setTextAreaValueAtCaret(
  element: HTMLTextAreaElement | null | undefined,
  value: string,
  caret: number,
): Promise<void> {
  expect(element, "expected textarea to exist").toBeTruthy();
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLTextAreaElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(element, value);
    element?.setSelectionRange(caret, caret);
    element?.dispatchEvent(new Event("input", { bubbles: true }));
  });
  await settle();
}

describe("ScheduledTasksView", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    if (typeof window.crypto?.randomUUID !== "function") {
      vi.stubGlobal("crypto", { randomUUID: () => "uuid-test" });
    }
    container = document.createElement("div");
    document.body.appendChild(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
    vi.unstubAllGlobals();
    document.body.innerHTML = "";
  });

  async function renderView(
    runtimeClient: RuntimeClient,
    props: { onOpenThread?: (threadId: string) => void } = {},
  ): Promise<void> {
    await act(async () => {
      root = createRoot(container);
      root.render(
        <ScheduledTasksView
          runtimeClient={runtimeClient}
          desktopReady
          onOpenThread={props.onOpenThread ?? (() => {})}
        />,
      );
    });
    await settle();
  }

  it("renders the empty state with both create entries", async () => {
    await renderView(createRuntimeClientMock());

    expect(document.body.textContent).toContain("定时任务");
    expect(document.body.textContent).toContain("Awaken 退出期间不会运行");
    expect(document.body.textContent).toContain("还没有定时任务");
    const empty = document.querySelector(".aw-scheduled-task-list__empty");
    expect(empty?.textContent).toContain("创建任务");
    expect(empty?.textContent).toContain("从模板创建");

    await click(buttonByText("创建任务"));
    expect(document.body.textContent).toContain("创建定时任务");
  });

  it("labels configured profiles by their actual model IDs", async () => {
    const kimiStandard: RuntimeModelProfileSummary = {
      ...readyProfile,
      id: "profile-kimi-standard",
      providerId: "kimi-coding-plan",
      displayName: "Kimi Coding Plan",
      modelId: "kimi-for-coding",
    };
    const kimiK3: RuntimeModelProfileSummary = {
      ...kimiStandard,
      id: "profile-kimi-k3",
      modelId: "k3",
      isDefault: false,
    };
    await renderView(
      createRuntimeClientMock({
        listModelProfiles: vi.fn(async () => [kimiStandard, kimiK3]),
      }),
    );

    await click(buttonByText("创建任务"));
    await click(document.querySelector(".aw-scheduled-task-editor__advanced > summary"));
    expect(document.querySelector('select[aria-label="模型 profile"]')).toBeNull();
    await click(buttonByAriaLabel("模型 profile"));
    const profileOptions = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.aw-select__menu [role="option"]'),
    );
    expect(profileOptions.map((option) => option.textContent?.trim())).toEqual([
      "每次运行时使用当前默认 profile",
      "kimi-for-coding",
      "k3",
    ]);
    expect(profileOptions.some((option) => /Chat|Responses|Messages/.test(option.textContent ?? "")))
      .toBe(false);
    expect(profileOptions.map((option) => option.dataset.value)).toEqual([
      "",
      "profile-kimi-standard",
      "profile-kimi-k3",
    ]);
    await click(profileOptions[2]);
    expect(buttonByAriaLabel("模型 profile")?.textContent).toContain("k3");
  });

  it("uses Chinese searchable timezone labels and the custom calendar", async () => {
    await renderView(createRuntimeClientMock());

    await click(buttonByText("创建任务"));
    await click(buttonByText("日程任务"));
    expect(document.querySelector('input[type="datetime-local"]')).toBeNull();

    await click(buttonByAriaLabel("开始时间"));
    expect(document.body.textContent).toContain("时间");
    expect(buttonByAriaLabel("上个月")).toBeTruthy();
    expect(buttonByAriaLabel("下个月")).toBeTruthy();

    await click(buttonByAriaLabel("时区"));
    expect(document.body.textContent).toContain("本机时区");
    expect(document.body.textContent).toContain("中国标准时间");
    expect(document.body.textContent).not.toContain("Asia/Shanghai");

    await setFieldValue(
      document.querySelector<HTMLInputElement>('input[aria-label="搜索时区"]'),
      "Tokyo",
    );
    expect(document.body.textContent).toContain("日本标准时间");
    expect(document.body.textContent).not.toContain("Asia/Tokyo");
  });

  it("renders rows with state badges and resolved project names", async () => {
    const tasks = [
      cronTask(),
      cronTask({ taskId: "task-2", name: "暂停的任务", enabled: false, state: "paused" }),
      cronTask({ taskId: "task-3", name: "运行中的任务", state: "running" }),
      cronTask({ taskId: "task-4", name: "孤儿任务", projectId: "project-gone" }),
    ];
    await renderView(
      createRuntimeClientMock({ listScheduledTasks: vi.fn(async () => tasks) }),
    );

    const text = document.body.textContent ?? "";
    expect(text).toContain("工作日早报");
    expect(text).toContain("工作日 09:30 · 中国标准时间（东8小时）");
    expect(text).toContain("Alpha");
    expect(text).toContain("启用");
    expect(text).toContain("已暂停");
    expect(text).toContain("运行中");
    // 未知项目回退为项目 ID
    expect(text).toContain("project-gone");
  });

  it("creates a task only when the editor submits", async () => {
    const created = cronTask({ taskId: "task-new" });
    const createScheduledTask = vi.fn(async () => created);
    const client = createRuntimeClientMock({ createScheduledTask });
    await renderView(client);

    expect(createScheduledTask).not.toHaveBeenCalled();
    await click(buttonByText("创建任务"));

    const nameInput = document.querySelector<HTMLInputElement>('input[aria-label="任务名称"]');
    await setFieldValue(nameInput, "新提醒");
    await click(buttonByAriaLabel("运行项目"));
    const projectOption = Array.from(
      document.querySelectorAll<HTMLButtonElement>('button[role="option"]'),
    ).find(
      (option) => option.textContent?.includes("Alpha"),
    );
    await click(projectOption);
    const content = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="提醒正文"]');
    await setFieldValue(content, "记得喝水");
    await click(buttonByText("保存任务"));

    expect(createScheduledTask).toHaveBeenCalledTimes(1);
    expect(createScheduledTask).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "新提醒",
        projectId: "project-1",
        action: { kind: "reminder", content: "记得喝水" },
        schedule: { kind: "cron", cron: "0 9 * * *", timezone: "Asia/Shanghai" },
      }),
    );
  });

  it("uses the home Composer slash menu and persists the selected Skill token", async () => {
    const created = cronTask({ taskId: "task-skill" });
    const createScheduledTask = vi.fn(async () => created);
    const listSkills = vi.fn(async () => [dailyReportSkill]);
    await renderView(
      createRuntimeClientMock({ createScheduledTask, listSkills }),
    );

    await click(buttonByText("创建任务"));
    await setFieldValue(
      document.querySelector<HTMLInputElement>('input[aria-label="任务名称"]'),
      "自动日报",
    );
    await click(buttonByAriaLabel("运行项目"));
    await click(document.querySelector<HTMLButtonElement>('button[role="option"]'));
    await click(buttonByText("Agent"));

    const prompt = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="Agent prompt"]',
    );
    await setFieldValue(prompt, "/daily");

    expect(listSkills).toHaveBeenCalledWith(["/workspace/alpha"]);
    expect(document.querySelector('[aria-label="Slash actions"]')).toBeTruthy();
    await click(buttonByText("日报 Skill"));

    expect(document.querySelector('[data-composer-token-id="skill:daily-report:project"]'))
      .toBeTruthy();
    await click(buttonByText("保存任务"));

    expect(createScheduledTask).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "自动日报",
        projectId: "project-1",
        action: {
          kind: "agent",
          content: "",
          composerTokens: [
            {
              id: "skill:daily-report:project",
              type: "skill",
              label: "日报 Skill",
              payload: {
                skillId: "daily-report",
                sourceOrigin: "project",
                skillFilePath: "/workspace/alpha/.awaken/skills/daily-report/SKILL.md",
                rootDir: "/workspace/alpha/.awaken/skills/daily-report",
              },
            },
          ],
        },
      }),
    );
  });

  it("opens a scheduled Skill menu at an inserted caret and preserves mixed suffix text", async () => {
    const created = cronTask({ taskId: "task-middle-skill" });
    const createScheduledTask = vi.fn(async () => created);
    await renderView(
      createRuntimeClientMock({
        createScheduledTask,
        listSkills: vi.fn(async () => [dailyReportSkill]),
      }),
    );

    await click(buttonByText("创建任务"));
    await setFieldValue(
      document.querySelector<HTMLInputElement>('input[aria-label="任务名称"]'),
      "句中技能任务",
    );
    await click(buttonByAriaLabel("运行项目"));
    await click(document.querySelector<HTMLButtonElement>('button[role="option"]'));
    await click(buttonByText("Agent"));

    const prompt = document.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="Agent prompt"]',
    );
    await setTextAreaValueAtCaret(prompt, "中文/dailyEnglish123", 8);
    expect(document.querySelector('[aria-label="Slash actions"]')).toBeNull();

    await setTextAreaValueAtCaret(prompt, "中文 /dailyEnglish123", 9);

    expect(document.querySelector('[aria-label="Slash actions"]')).toBeTruthy();
    await click(buttonByText("日报 Skill"));
    const token = document.querySelector<HTMLElement>(
      '[data-composer-token-id="skill:daily-report:project"]',
    );
    expect(token?.previousSibling?.textContent).toBe("中文 ");
    expect(token?.nextSibling?.textContent).toBe("English123");

    await click(buttonByText("保存任务"));
    expect(createScheduledTask).toHaveBeenCalledWith(
      expect.objectContaining({
        action: expect.objectContaining({
          kind: "agent",
          content: "English123",
          composerTokens: [expect.objectContaining({ leadingText: "中文 " })],
        }),
      }),
    );
  });

  it("uses the Plugin mention catalog and persists a scheduled Plugin token", async () => {
    const created = cronTask({ taskId: "task-plugin" });
    const createScheduledTask = vi.fn(async () => created);
    const plugin: RuntimePlugin = {
      name: "acme.tools",
      version: "1.0.0",
      description: "Acme workflows",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/acme" },
      digest: "digest-v1",
      skillCount: 1,
      serverCount: 1,
    };
    const listPlugins = vi.fn(async () => [plugin]);
    await renderView(
      createRuntimeClientMock({ createScheduledTask, listPlugins }),
    );

    await click(buttonByText("创建任务"));
    await setFieldValue(
      document.querySelector<HTMLInputElement>('input[aria-label="任务名称"]'),
      "插件任务",
    );
    await click(buttonByAriaLabel("运行项目"));
    await click(document.querySelector<HTMLButtonElement>('button[role="option"]'));
    await click(buttonByText("Agent"));
    await setFieldValue(
      document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Agent prompt"]'),
      "@acme",
    );

    expect(listPlugins).toHaveBeenCalledTimes(1);
    await click(buttonByText("acme.tools"));
    expect(document.querySelector('[data-composer-token-id="plugin:acme.tools:digest-v1"]'))
      .toBeTruthy();
    await click(buttonByText("保存任务"));
    expect(createScheduledTask).toHaveBeenCalledWith(
      expect.objectContaining({
        action: {
          kind: "agent",
          content: "",
          composerTokens: [
            {
              id: "plugin:acme.tools:digest-v1",
              type: "plugin",
              label: "acme.tools",
              payload: { pluginName: "acme.tools", pluginDigest: "digest-v1" },
            },
          ],
        },
      }),
    );
  });

  it("offers and persists the Computer Use capability in a scheduled Agent task", async () => {
    const created = cronTask({ taskId: "task-computer-use" });
    const createScheduledTask = vi.fn(async () => created);
    await renderView(createRuntimeClientMock({ createScheduledTask }));

    await click(buttonByText("创建任务"));
    await setFieldValue(
      document.querySelector<HTMLInputElement>('input[aria-label="任务名称"]'),
      "计算机操作任务",
    );
    await click(buttonByAriaLabel("运行项目"));
    await click(document.querySelector<HTMLButtonElement>('button[role="option"]'));
    await click(buttonByText("Agent"));
    await setFieldValue(
      document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Agent prompt"]'),
      "@computer",
    );

    await click(buttonByText("Computer Use"));
    expect(document.querySelector('[data-composer-token-id="capability:computer-use"]'))
      .toBeTruthy();
    await click(buttonByText("保存任务"));
    expect(createScheduledTask).toHaveBeenCalledWith(
      expect.objectContaining({
        action: {
          kind: "agent",
          content: "",
          composerTokens: [
            {
              id: "capability:computer-use",
              type: "capability",
              label: "Computer Use",
              payload: { capabilityId: "computer-use" },
            },
          ],
        },
      }),
    );
  });

  it("refreshes an open Plugin mention menu when the catalog revision changes", async () => {
    let catalogHandler: ((event: { revision: number; kind: string }) => void) | undefined;
    const plugin: RuntimePlugin = {
      name: "fresh.plugin",
      enabled: true,
      source: { kind: "local_path", path: "/tmp/fresh" },
      digest: "fresh-digest",
      skillCount: 1,
      serverCount: 0,
    };
    const listPlugins = vi
      .fn<() => Promise<RuntimePlugin[]>>()
      .mockResolvedValueOnce([])
      .mockResolvedValue([plugin]);
    await renderView(
      createRuntimeClientMock({
        listPlugins,
        subscribeComposerCatalogEvents: vi.fn((handler) => {
          catalogHandler = handler;
          return () => {};
        }),
      }),
    );
    await click(buttonByText("创建任务"));
    await click(buttonByAriaLabel("运行项目"));
    await click(document.querySelector<HTMLButtonElement>('button[role="option"]'));
    await click(buttonByText("Agent"));
    await setFieldValue(
      document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Agent prompt"]'),
      "@fresh",
    );
    expect(document.body.textContent).not.toContain("fresh.plugin");
    expect(document.body.textContent).toContain("没有匹配的能力或 Plugin");
    await act(async () => catalogHandler?.({ revision: 2, kind: "plugins" }));
    await settle();
    expect(listPlugins).toHaveBeenCalledTimes(2);
    expect(buttonByText("fresh.plugin")).toBeTruthy();
  });

  it("uses a searchable project dropdown instead of expanding project cards", async () => {
    const projects: RuntimeProject[] = Array.from({ length: 12 }, (_, index) => ({
      ...project,
      projectId: `project-${index + 1}`,
      displayName: `测试项目${String(index + 1).padStart(2, "0")}`,
      workspaceRoot: `/workspace/project-${index + 1}`,
    }));
    await renderView(
      createRuntimeClientMock({ listProjects: vi.fn(async () => projects) }),
    );

    await click(buttonByText("创建任务"));
    expect(document.querySelector('input[type="radio"][name$="-project"]')).toBeNull();
    const trigger = buttonByAriaLabel("运行项目");
    expect(trigger?.getAttribute("aria-expanded")).toBe("false");

    await click(trigger);
    expect(buttonByAriaLabel("运行项目")?.getAttribute("aria-expanded")).toBe("true");
    expect(document.body.textContent).toContain("12 个项目");
    expect(document.querySelector('[role="listbox"][aria-label="运行项目列表"]')).toBeTruthy();

    await setFieldValue(
      document.querySelector<HTMLInputElement>('input[aria-label="搜索项目"]'),
      "项目11",
    );
    const options = Array.from(
      document.querySelectorAll<HTMLButtonElement>('button[role="option"]'),
    );
    expect(options).toHaveLength(1);
    expect(options[0]?.textContent).toContain("测试项目11");

    await click(options[0]);
    expect(buttonByAriaLabel("运行项目")?.textContent).toContain("测试项目11");
    expect(buttonByAriaLabel("运行项目")?.getAttribute("aria-expanded")).toBe("false");
  });

  it("prefills from a template without calling create", async () => {
    const createScheduledTask = vi.fn(async () => cronTask());
    await renderView(createRuntimeClientMock({ createScheduledTask }));

    await click(buttonByText("从模板创建"));
    expect(document.body.textContent).toContain("循环任务");
    await click(buttonByText("使用模板"));

    // 模板选择只打开预填的编辑器，绝不发创建请求。
    expect(createScheduledTask).not.toHaveBeenCalled();
    const nameInput = document.querySelector<HTMLInputElement>('input[aria-label="任务名称"]');
    expect(nameInput?.value).toBe("工作日科技新闻早报");
  });

  it("pauses optimistically and rolls back on failure", async () => {
    const task = cronTask();
    const pauseScheduledTask = vi.fn(async () => {
      throw new Error(
        "Error invoking remote method 'awaken:pause-scheduled-task': RuntimeDomainError: VERSION_CONFLICT: task was modified; reload and retry",
      );
    });
    const listScheduledTasks = vi.fn(async () => [task]);
    await renderView(
      createRuntimeClientMock({ listScheduledTasks, pauseScheduledTask }),
    );

    const toggle = document.querySelector<HTMLButtonElement>('button[role="switch"]');
    expect(toggle?.getAttribute("aria-checked")).toBe("true");
    await click(toggle);

    expect(pauseScheduledTask).toHaveBeenCalledWith("task-1", 2);
    // 失败后按真实状态回滚并提示版本冲突。
    expect(document.querySelector<HTMLButtonElement>('button[role="switch"]')?.getAttribute("aria-checked")).toBe("true");
    expect(document.body.textContent).toContain("任务已被其他地方修改，请刷新后重试。");
  });

  it("resumes a paused task with its current version", async () => {
    const paused = cronTask({ enabled: false, state: "paused" });
    const resumed = cronTask();
    const resumeScheduledTask = vi.fn(async () => resumed);
    await renderView(
      createRuntimeClientMock({
        listScheduledTasks: vi.fn(async () => [paused]),
        resumeScheduledTask,
      }),
    );

    await click(document.querySelector<HTMLButtonElement>('button[role="switch"]'));
    expect(resumeScheduledTask).toHaveBeenCalledWith("task-1", 2);
  });

  it("runs a task now after confirmation and shows the already-running conflict", async () => {
    const runScheduledTaskNow = vi.fn(async () => {
      throw new Error(
        "Error invoking remote method 'awaken:run-scheduled-task-now': RuntimeDomainError: TASK_ALREADY_RUNNING: task already has an active execution",
      );
    });
    await renderView(
      createRuntimeClientMock({
        listScheduledTasks: vi.fn(async () => [cronTask()]),
        runScheduledTaskNow,
      }),
    );

    await click(buttonByAriaLabel("工作日早报 更多操作"));
    await click(buttonByText("立即执行"));
    expect(runScheduledTaskNow).not.toHaveBeenCalled();
    await click(buttonByText("确认执行"));

    expect(runScheduledTaskNow).toHaveBeenCalledWith("task-1", expect.any(String));
    expect(document.body.textContent).toContain("该任务已有正在进行的执行，请等待结束后再试。");
  });

  it("deletes a task after confirmation", async () => {
    const deleteScheduledTask = vi.fn(async () => undefined);
    const listScheduledTasks = vi.fn(async () => [cronTask()]);
    await renderView(
      createRuntimeClientMock({ listScheduledTasks, deleteScheduledTask }),
    );

    await click(buttonByAriaLabel("工作日早报 更多操作"));
    await click(buttonByText("删除"));
    expect(deleteScheduledTask).not.toHaveBeenCalled();
    await click(buttonByText("确认删除"));

    expect(deleteScheduledTask).toHaveBeenCalledWith("task-1", 2, expect.any(String));
  });

  it("refreshes the task list silently on scheduled task events", async () => {
    let eventHandler: ((event: ScheduledTaskEvent) => void) | undefined;
    const listScheduledTasks = vi.fn(async () => [cronTask()]);
    await renderView(
      createRuntimeClientMock({
        listScheduledTasks,
        subscribeScheduledTaskEvents: vi.fn((handler) => {
          eventHandler = handler;
          return () => {};
        }),
      }),
    );

    expect(listScheduledTasks).toHaveBeenCalledTimes(1);
    await act(async () => {
      eventHandler?.({ type: "execution_finished", taskId: "task-1", status: "success" });
    });
    await settle();

    expect(listScheduledTasks).toHaveBeenCalledTimes(2);
  });

  it("opens the execution thread from history", async () => {
    const onOpenThread = vi.fn();
    await renderView(
      createRuntimeClientMock({
        listScheduledTasks: vi.fn(async () => [cronTask()]),
        listScheduledTaskExecutions: vi.fn(async () => [
          {
            executionId: "exec-1",
            taskId: "task-1",
            taskVersion: 2,
            trigger: "manual" as const,
            scheduledFor: "2026-07-27T01:00:00Z",
            startedAt: "2026-07-27T01:00:01Z",
            finishedAt: "2026-07-27T01:01:00Z",
            status: "success" as const,
            threadId: "thread-1",
          },
        ]),
      }),
      { onOpenThread },
    );

    await click(buttonByAriaLabel("工作日早报 更多操作"));
    await click(buttonByText("执行记录"));
    expect(document.body.textContent).toContain("执行记录 · 工作日早报");
    expect(document.body.textContent).toContain("手动");

    await click(buttonByText("打开线程"));
    expect(onOpenThread).toHaveBeenCalledWith("thread-1");
  });
});
