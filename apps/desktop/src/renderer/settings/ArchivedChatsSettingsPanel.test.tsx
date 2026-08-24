// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeProject, RuntimeThread } from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import {
  ArchivedChatsSettingsPanel,
  filterArchivedChatGroups,
  groupArchivedThreadsByProject,
  unknownProjectGroupLabel,
} from "./ArchivedChatsSettingsPanel";

function makeThread(overrides: Partial<RuntimeThread> = {}): RuntimeThread {
  return {
    threadId: "thread-1",
    label: "归档会话",
    archived: true,
    archivedAt: "2026-08-20T10:20:30Z",
    workspace: { workspaceRoot: "/tmp", writableRoots: ["/tmp"] },
    createdAt: "2026-08-01T00:00:00Z",
    updatedAt: "2026-08-02T00:00:00Z",
    ...overrides,
  };
}

function makeProject(overrides: Partial<RuntimeProject> = {}): RuntimeProject {
  return {
    projectId: "project-1",
    workspaceRoot: "/tmp/project-1",
    folders: [],
    displayName: "项目一",
    state: "active",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    version: 1,
    ...overrides,
  };
}

describe("unknownProjectGroupLabel", () => {
  it("uses the last 8 characters of the project id", () => {
    expect(unknownProjectGroupLabel("abcdefghij")).toBe("未知项目（cdefghij）");
  });
});

describe("groupArchivedThreadsByProject", () => {
  it("orders active projects, removed projects, conversations, then unknown projects", () => {
    const threads = [
      makeThread({ threadId: "t-unknown-b", projectId: "unknown-zzzzzzzz" }),
      makeThread({ threadId: "t-conversation", projectId: undefined }),
      makeThread({ threadId: "t-removed", projectId: "project-removed" }),
      makeThread({ threadId: "t-active-2", projectId: "project-active-2" }),
      makeThread({ threadId: "t-active-1", projectId: "project-active-1" }),
      makeThread({ threadId: "t-unknown-a", projectId: "unknown-aaaaaaaa" }),
    ];
    const projects = [
      makeProject({ projectId: "project-active-1", displayName: "active 一" }),
      makeProject({ projectId: "project-active-2", displayName: "active 二" }),
      makeProject({ projectId: "project-removed", displayName: "removed 项目", state: "removed" }),
      // 没有归档会话的空项目不应渲染。
      makeProject({ projectId: "project-empty", displayName: "空项目" }),
    ];

    const groups = groupArchivedThreadsByProject(threads, projects);

    expect(groups.map((group) => group.id)).toEqual([
      "project-active-1",
      "project-active-2",
      "project-removed",
      "conversation",
      "unknown-aaaaaaaa",
      "unknown-zzzzzzzz",
    ]);
    expect(groups.map((group) => group.label)).toEqual([
      "active 一",
      "active 二",
      "removed 项目",
      "对话",
      "未知项目（aaaaaaaa）",
      "未知项目（zzzzzzzz）",
    ]);
    // 组内保持 Core 返回顺序，面板不重排。
    expect(groups[0].rows.map((row) => row.threadId)).toEqual(["t-active-1"]);
  });

  it("falls back to updatedAt when archivedAt is absent", () => {
    const groups = groupArchivedThreadsByProject(
      [makeThread({ archivedAt: undefined, updatedAt: "2026-07-12T17:36:00Z" })],
      [],
    );
    const date = new Date("2026-07-12T17:36:00Z");
    expect(groups[0].rows[0].archivedAtLabel).toBe(date.toLocaleDateString());
    expect(groups[0].rows[0].archivedAtTitle).toBe(date.toLocaleString());
  });

  it("keeps unresolvable non-empty projectIds as separate groups", () => {
    const groups = groupArchivedThreadsByProject(
      [
        makeThread({ threadId: "t-1", projectId: "unknown-1" }),
        makeThread({ threadId: "t-2", projectId: "unknown-2" }),
      ],
      [],
    );
    expect(groups).toHaveLength(2);
  });
});

describe("filterArchivedChatGroups", () => {
  const groups = groupArchivedThreadsByProject(
    [
      makeThread({ threadId: "t-1", label: "Alpha 报告", projectId: "project-1" }),
      makeThread({ threadId: "t-2", label: "beta 讨论", projectId: "project-1" }),
      makeThread({ threadId: "t-3", label: "GAMMA", projectId: undefined }),
    ],
    [makeProject({ projectId: "project-1", displayName: "项目一" })],
  );

  it("matches titles case-insensitively", () => {
    const result = filterArchivedChatGroups(groups, "ALPHA", "all");
    expect(result).toHaveLength(1);
    expect(result[0].rows.map((row) => row.threadId)).toEqual(["t-1"]);

    const lower = filterArchivedChatGroups(groups, "gamma", "all");
    expect(lower[0].rows.map((row) => row.threadId)).toEqual(["t-3"]);
  });

  it("filters by project and drops empty groups", () => {
    const result = filterArchivedChatGroups(groups, "", "conversation");
    expect(result.map((group) => group.id)).toEqual(["conversation"]);

    const none = filterArchivedChatGroups(groups, "不存在的词", "all");
    expect(none).toEqual([]);
  });
});

type MockClient = {
  listThreads: ReturnType<typeof vi.fn>;
  listProjects: ReturnType<typeof vi.fn>;
  setThreadArchived: ReturnType<typeof vi.fn>;
  deleteThread: ReturnType<typeof vi.fn>;
};

function makeClient(overrides: Partial<MockClient> = {}): RuntimeClient {
  return {
    listThreads: vi.fn(async () => [makeThread({ projectId: "project-1" })]),
    listProjects: vi.fn(async () => [makeProject()]),
    setThreadArchived: vi.fn(async () => ({ ok: true as const, thread: makeThread() })),
    deleteThread: vi.fn(async () => ({ ok: true as const })),
    ...overrides,
  } as unknown as RuntimeClient;
}

function buttonByText(scope: ParentNode, text: string): HTMLButtonElement {
  const button = [...scope.querySelectorAll("button")].find(
    (candidate) => candidate.textContent?.trim() === text,
  );
  if (!button) {
    throw new Error(`button not found: ${text}`);
  }
  return button;
}

describe("ArchivedChatsSettingsPanel", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
  });

  async function renderPanel(
    client: RuntimeClient,
    onThreadsChanged = vi.fn(),
    onToast = vi.fn(),
  ) {
    await act(async () => {
      root.render(
        <ArchivedChatsSettingsPanel
          runtimeClient={client}
          onThreadsChanged={onThreadsChanged}
          onToast={onToast}
        />,
      );
    });
    return onThreadsChanged;
  }

  it("loads archived threads and projects (including removed) on mount", async () => {
    const client = makeClient();
    await renderPanel(client);

    expect(client.listThreads).toHaveBeenCalledWith("archived");
    expect(client.listProjects).toHaveBeenCalledWith(true);
    expect(container.textContent).toContain("项目一");
    expect(container.textContent).toContain("归档会话");
    expect(container.textContent).toContain("取消归档");
  });

  it("renders a full search field beside a locally sized project filter", async () => {
    await renderPanel(makeClient());

    const search = container.querySelector<HTMLInputElement>(
      '.aw-archived-chats__search input[aria-label="搜索已归档聊天"]',
    );
    expect(search?.type).toBe("search");
    expect(search?.placeholder).toBe("搜索已归档聊天");
    expect(
      container.querySelector(".aw-select.aw-archived-chats__project-filter"),
    ).not.toBeNull();
  });

  it("shows the empty state when nothing is archived", async () => {
    const client = makeClient({ listThreads: vi.fn(async () => []) });
    await renderPanel(client);

    expect(container.textContent).toContain("还没有已归档的聊天");
  });

  it("shows a retryable error state instead of faking an empty library", async () => {
    const listThreads = vi
      .fn()
      .mockRejectedValueOnce(new Error("网络错误"))
      .mockResolvedValueOnce([makeThread()]);
    const client = makeClient({ listThreads });
    await renderPanel(client);

    expect(container.textContent).toContain("网络错误");
    expect(container.textContent).not.toContain("还没有已归档的聊天");

    await act(async () => {
      buttonByText(container, "重试").click();
    });
    expect(container.textContent).toContain("归档会话");
  });

  it("unarchives a row, refreshes the sidebar and keeps other rows", async () => {
    const client = makeClient();
    const onToast = vi.fn();
    const onThreadsChanged = await renderPanel(client, vi.fn(), onToast);

    await act(async () => {
      buttonByText(container, "取消归档").click();
    });

    expect(client.setThreadArchived).toHaveBeenCalledWith("thread-1", false);
    expect(onThreadsChanged).toHaveBeenCalledTimes(1);
    expect(container.textContent).not.toContain("归档会话");
    expect(container.textContent).not.toContain("已取消归档");
    expect(onToast).toHaveBeenCalledWith("已取消归档");
  });

  it("explains the removed-project case after unarchive", async () => {
    const client = makeClient({
      listThreads: vi.fn(async () => [makeThread({ projectId: "project-1" })]),
      listProjects: vi.fn(async () => [makeProject({ state: "removed" })]),
    });
    const onToast = vi.fn();
    await renderPanel(client, vi.fn(), onToast);

    await act(async () => {
      buttonByText(container, "取消归档").click();
    });

    expect(container.textContent).not.toContain("已取消归档；重新添加项目后可在侧栏查看");
    expect(onToast).toHaveBeenCalledWith("已取消归档；重新添加项目后可在侧栏查看");
  });

  it("keeps the row and shows the failure notice when unarchive fails", async () => {
    const client = makeClient({
      setThreadArchived: vi.fn(async () => ({
        ok: false as const,
        code: "THREAD_BUSY",
        message: "请先停止正在运行的任务",
      })),
    });
    const onToast = vi.fn();
    await renderPanel(client, vi.fn(), onToast);

    await act(async () => {
      buttonByText(container, "取消归档").click();
    });

    expect(container.textContent).toContain("归档会话");
    expect(container.textContent).not.toContain("请先停止正在运行的任务");
    expect(onToast).toHaveBeenCalledWith("请先停止正在运行的任务");
  });

  it("keeps the row when the unarchive bridge rejects", async () => {
    const client = makeClient({
      setThreadArchived: vi.fn(async () => {
        throw new Error("运行时连接已断开");
      }),
    });
    const onToast = vi.fn();
    await renderPanel(client, vi.fn(), onToast);

    await act(async () => {
      buttonByText(container, "取消归档").click();
    });

    expect(container.textContent).toContain("归档会话");
    expect(container.textContent).not.toContain("运行时连接已断开");
    expect(onToast).toHaveBeenCalledWith("运行时连接已断开");
  });

  it("confirms deletion through the shared dialog and reloads", async () => {
    const client = makeClient();
    const onThreadsChanged = await renderPanel(client);

    await act(async () => {
      container.querySelector<HTMLButtonElement>('[aria-label="删除会话"]')!.click();
    });
    expect(document.body.textContent).toContain("此操作无法撤销");
    expect(document.body.textContent).toContain("对话产生的文件不会被删除");

    await act(async () => {
      buttonByText(document.body, "删除会话").click();
    });

    expect(client.deleteThread).toHaveBeenCalledWith("thread-1");
    expect(onThreadsChanged).toHaveBeenCalled();
  });

  it("does not delete when the confirmation is cancelled", async () => {
    const client = makeClient();
    await renderPanel(client);

    await act(async () => {
      container.querySelector<HTMLButtonElement>('[aria-label="删除会话"]')!.click();
    });
    await act(async () => {
      buttonByText(document.body, "取消").click();
    });

    expect(client.deleteThread).not.toHaveBeenCalled();
  });

  it("keeps the delete dialog open when the bridge rejects", async () => {
    const client = makeClient({
      deleteThread: vi.fn(async () => {
        throw new Error("删除通道不可用");
      }),
    });
    await renderPanel(client);

    await act(async () => {
      container.querySelector<HTMLButtonElement>('[aria-label="删除会话"]')!.click();
    });
    await act(async () => {
      buttonByText(document.body, "删除会话").click();
    });

    expect(document.body.textContent).toContain("删除通道不可用");
    expect(document.body.textContent).toContain("此操作无法撤销");
  });

  it("refreshes once and explains when the record is already gone", async () => {
    const client = makeClient({
      deleteThread: vi.fn(async () => ({
        ok: false as const,
        code: "THREAD_NOT_FOUND",
        message: "not found",
      })),
    });
    await renderPanel(client);

    await act(async () => {
      container.querySelector<HTMLButtonElement>('[aria-label="删除会话"]')!.click();
    });
    await act(async () => {
      buttonByText(document.body, "删除会话").click();
    });

    expect(container.textContent).toContain("该记录已不存在，列表已刷新。");
    // 初次载入一次 + 删除后刷新一次。
    expect(client.listThreads).toHaveBeenCalledTimes(2);
  });
});
