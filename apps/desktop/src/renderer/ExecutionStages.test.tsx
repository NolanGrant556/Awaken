// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AwakenToolTraceGroup,
  AwakenToolTraceLine,
} from "@awaken/design-system";
import {
  activityBatchSummary,
  toolTracePresentation,
} from "./toolTracePresentation";
import {
  projectVisibleNarrativeItems,
  type NarrativeTurnBodySegment,
} from "./workbenchPresentation";
import { traceFileTarget } from "./traceFileNavigation";
import { toolResultPatch } from "./tracePatch";
import { createWorkbenchDisclosureStore } from "./workbenchDisclosureStore";
import { StoredThoughtTrace } from "./StoredExecutionDisclosures";
import {
  createInitialRuntimeState,
  reduceRuntimeState,
} from "./runtime/reducer";
import { useNarrativeRenderer } from "./useNarrativeRenderer";
import type { RunNarrativeItem } from "./run-narrative";

describe("Cursor-style execution stages", () => {
  it("keeps submitted planning active until the first real output, including committed placeholders", () => {
    const items: RunNarrativeItem[] = [
      {
        kind: "planning",
        id: "planning:r",
        text: "Planning next moves",
        status: "done",
      },
    ];
    expect(
      projectVisibleNarrativeItems({ items, isBusy: true })[0],
    ).toMatchObject({ status: "active" });
    expect(projectVisibleNarrativeItems({ items, isBusy: false })).toEqual([]);
  });

  it("distinguishes a newline-only edit and empty command output", () => {
    const patch = toolResultPatch({
      title: "Edit",
      status: "done",
      content: JSON.stringify({
        file_path: "a.ts",
        hunks: [{ old_text: "old", new_text: "old\n" }],
      }),
    });
    expect(patch?.patch).toContain("-old\n\\ No newline at end of file\n+old");
    expect(
      toolTracePresentation({
        title: "Bash",
        args: '{"command":"true"}',
        content: "Tool returned empty content",
        status: "done",
      }).detailCard?.sections,
    ).toContainEqual({
      label: "输出",
      content: "命令已结束，无输出。",
      format: "text",
    });
  });
  it("projects real Edit and created Write receipts without inventing overwritten before content", () => {
    const before =
      Array.from(
        { length: 100 },
        (_, i) => `export const value${i + 1} = ${i + 1};`,
      ).join("\n") + "\n";
    const after = before.replace("value40 = 40", "value40 = 140");
    const record = {
      title: "Edit",
      status: "done" as const,
      content: JSON.stringify({
        file_path: "sample.ts",
        hunks: [{ old_text: before, new_text: after }],
      }),
    };
    const patch = toolResultPatch(record);
    expect(patch?.patch).toContain("@@ -37,7 +37,7 @@");
    expect(patch?.patch).toContain("-export const value40 = 40;");
    expect(patch?.patch).toContain("+export const value40 = 140;");
    expect(toolResultPatch({ ...record, status: "active" })).toBeUndefined();
    expect(
      toolResultPatch({
        title: "Write",
        status: "done",
        args: '{"content":"new"}',
        content: '{"action":"overwritten","file_path":"a.ts"}',
      }),
    ).toBeUndefined();
    expect(
      toolResultPatch({
        title: "Write",
        status: "done",
        args: '{"content":"new"}',
        content: '{"action":"created","file_path":"a.ts"}',
      })?.patch,
    ).toContain("--- /dev/null");
    expect(
      toolResultPatch({ ...record, content: record.content.slice(0, 80) }),
    ).toBeUndefined();
  });
  let container: HTMLDivElement;
  let root: ReturnType<typeof createRoot>;
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
    container.remove();
  });

  it("counts successful distinct files and calls, excludes previews and incomplete totals", () => {
    const patch = {
      patch: "--- a/a.ts\n+++ b/a.ts\n@@ -1 +1 @@\n-old\n+new",
      affectedPaths: ["a.ts"],
    };
    const actions = [
      {
        id: "read1",
        kind: "tool" as const,
        title: "任意翻译",
        toolName: "Read",
        paths: ["./a.ts"],
        status: "done" as const,
      },
      {
        id: "read2",
        kind: "tool" as const,
        title: "任意翻译",
        toolName: "Read",
        paths: ["a.ts"],
        status: "done" as const,
      },
      {
        id: "edit1",
        kind: "tool" as const,
        title: "任意翻译",
        toolName: "Edit",
        paths: ["a.ts"],
        status: "done" as const,
        patch,
      },
      {
        id: "edit2",
        kind: "tool" as const,
        title: "任意翻译",
        toolName: "Edit",
        paths: ["b.ts"],
        status: "active" as const,
        patch,
      },
    ];
    expect(activityBatchSummary([...actions, actions[2]])).toEqual({
      label: "修改 1 个文件、探索 1 个文件",
      added: 1,
      removed: 1,
      detail: undefined,
    });
    expect(
      activityBatchSummary([
        ...actions,
        {
          ...actions[2],
          id: "truncated",
          patch: { ...patch, truncated: true },
        },
      ]).detail,
    ).toBeUndefined();
    expect(
      activityBatchSummary([
        { kind: "tool", title: "Read 100 files", status: "done" },
      ]).label,
    ).toBe("使用 1 次工具或技能");
  });

  it("keeps tool and reasoning disclosure choices through stage growth and final wrapping", async () => {
    const store = createWorkbenchDisclosureStore();
    const state = createInitialRuntimeState();
    const tool: RunNarrativeItem = {
      kind: "tool",
      id: "tool:r:one",
      toolCallId: "one",
      title: "读取",
      detail: "已有记录",
      status: "done",
    };
    function Feed({
      items,
      final = false,
    }: {
      items: RunNarrativeItem[];
      final?: boolean;
    }) {
      const renderer = useNarrativeRenderer({
        runtimeState: state,
        appearanceTheme: "dark",
        platform: "macos",
        replayedNarrativeItemIds: new Set(),
        media: {} as never,
        subagentNowMs: 0,
        handleOpenSubagent: () => {},
        openMediaPreview: () => {},
        handleOpenDeliverablePreview: () => {},
        setNotice: () => {},
        workbenchDisclosureStore: store,
      });
      const segment: NarrativeTurnBodySegment = final
        ? {
            kind: "executionProcess",
            id: "execution:turn",
            items,
            title: "执行过程",
            open: false,
          }
        : { kind: "items", id: "items:turn", items };
      return (
        <>
          {renderer.renderTurnBodySegment(
            "turn",
            segment,
            { running: !final },
            `thread:thread:turn:turn:body:${segment.id}`,
          )}
        </>
      );
    }
    await act(async () => root.render(<Feed items={[tool]} />));
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>("button.tool-trace-line")!
        .click(),
    );
    expect(container.textContent).toContain("已有记录");
    const items: RunNarrativeItem[] = [
      tool,
      {
        kind: "thinking",
        id: "thought:r:two",
        text: "进一步分析",
        status: "active",
      },
    ];
    await act(async () => root.render(<Feed items={items} />));
    expect(container.querySelector(".tool-trace-group-heading")).toBeNull();
    expect(container.querySelector(".execution-activity-list")).not.toBeNull();
    expect(container.textContent).toContain("已有记录");
    expect(container.textContent).toContain("思考 中");
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>(".thought-trace-heading")!
        .click(),
    );
    await act(async () =>
      root.render(
        <Feed
          items={[tool, { ...items[1], status: "done" } as RunNarrativeItem]}
          final
        />,
      ),
    );
    const process = container.querySelector<HTMLButtonElement>(
      ".aw-disclosure-trigger",
    );
    expect(process).not.toBeNull();
    expect(process?.textContent).toBe("执行过程");
    expect(process?.textContent).not.toContain("使用 1 次工具或技能");
    await act(async () => process!.click());
    expect(container.textContent).toContain("进一步分析");
    expect(container.textContent).toContain("思考 片刻");
    expect(container.querySelector(".aw-trace-dots")).toBeNull();
  });

  it.each([
    { label: "durable elapsed", elapsedMs: 29_000, timing: { running: false }, expected: "执行时间 29 秒" },
    { label: "zero elapsed", elapsedMs: 0, timing: { running: false }, expected: "执行时间 0 秒" },
    { label: "current terminal timestamps", elapsedMs: undefined, timing: { running: false, startedAtMs: 1_000, completedAtMs: 18_000 }, expected: "执行时间 17 秒" },
    { label: "durable elapsed takes priority", elapsedMs: 29_000, timing: { running: false, startedAtMs: 1_000, completedAtMs: 18_000 }, expected: "执行时间 29 秒" },
    { label: "old history without timing", elapsedMs: undefined, timing: { running: false }, expected: "执行过程" },
    { label: "incomplete historical timing", elapsedMs: undefined, timing: { running: false, startedAtMs: 1_000 }, expected: "执行过程" },
    { label: "invalid historical timing", elapsedMs: Number.NaN, timing: { running: false }, expected: "执行过程" },
  ])("uses elapsed time for the outer process title: $label", async ({ elapsedMs, timing, expected }) => {
    const store = createWorkbenchDisclosureStore();
    const state = createInitialRuntimeState();
    function Feed() {
      const renderer = useNarrativeRenderer({ runtimeState: state, appearanceTheme: "light", platform: "macos",
        replayedNarrativeItemIds: new Set(), media: {} as never, subagentNowMs: 0,
        handleOpenSubagent: () => {}, openMediaPreview: () => {}, handleOpenDeliverablePreview: () => {},
        setNotice: () => {}, workbenchDisclosureStore: store });
      return <>{renderer.renderTurnBodySegment("turn", { kind: "executionProcess", id: "execution:turn",
        title: "执行过程", elapsedMs, open: false, items: [
          { kind: "tool", id: "tool:r:one", toolCallId: "one", title: "Bash", detail: "保留工具详情", status: "done" },
        ] }, timing, "thread:fixture:turn:turn:body:execution:turn")}</>;
    }
    await act(async () => root.render(<Feed />));
    const heading = container.querySelector<HTMLButtonElement>(".aw-disclosure-trigger")!;
    expect(heading.textContent).toBe(expected);
    expect(container.querySelectorAll('[role="timer"]')).toHaveLength(expected === "执行过程" ? 0 : 1);
    expect(heading.getAttribute("aria-expanded")).toBe("false");
    await act(async () => heading.click());
    expect(heading.textContent).toBe(expected);
    expect(container.querySelector(".execution-activity-list")).not.toBeNull();
    await act(async () => root.render(null));
    await act(async () => root.render(<Feed />));
    expect(container.querySelector(".aw-disclosure-trigger")?.textContent).toBe(expected);
    expect(container.querySelector(".aw-disclosure-trigger")?.getAttribute("aria-expanded")).toBe("true");
  });

  it("preserves explicit thinking expansion across remount and omits empty disclosures", async () => {
    const store = createWorkbenchDisclosureStore();
    const thought = (
      <StoredThoughtTrace
        store={store}
        disclosureKey="thread:run:thinking"
        text="思考正文"
        status="active"
      />
    );
    await act(async () => root.render(thought));
    expect(container.textContent).not.toContain("思考正文");
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>(".thought-trace-heading")!
        .click(),
    );
    await act(async () => root.render(null));
    await act(async () => root.render(thought));
    expect(container.textContent).toContain("思考正文");
    await act(async () =>
      root.render(
        <StoredThoughtTrace
          store={store}
          disclosureKey="empty"
          text=""
          status="active"
        />,
      ),
    );
    expect(container.querySelector("button")).toBeNull();
    expect(container.querySelector('[role="status"]')).not.toBeNull();
  });

  it("keeps current activity and failure visible when a running stage is collapsed", async () => {
    await act(async () =>
      root.render(
        <AwakenToolTraceGroup
          actions={[]}
          entryCount={3}
          label="运行 1 条命令"
          activityStatus="active"
          currentActivity="正在构建 · 另有 1 项进行中"
          attention="检查失败：目标不存在"
        >
          <span>hidden body</span>
        </AwakenToolTraceGroup>,
      ),
    );
    expect(container.textContent).toContain("另有 1 项进行中");
    expect(container.textContent).toContain("检查失败");
    expect(container.textContent).not.toContain("hidden body");
  });

  it("renders line-numbered colored diff without interpreting source as markup", async () => {
    await act(async () =>
      root.render(
        <AwakenToolTraceLine
          title="编辑 a.ts"
          defaultOpen
          detailCard={{
            title: "变更",
            sections: [
              {
                label: "变更",
                format: "diff",
                content:
                  '--- a/a.ts\n+++ b/a.ts\n@@ -8 +8 @@\n-const a = 1\n+const a = "<script>"\n--- a/b.ts\n+++ b/b.ts\n@@ -40 +40 @@\n-old\n+new',
              },
            ],
          }}
        />,
      ),
    );
    expect(
      container.querySelector('tr[data-kind="remove"] .trace-diff-number')
        ?.textContent,
    ).toBe("8");
    expect(
      container.querySelectorAll('tr[data-kind="add"] .trace-diff-number')[1]
        ?.textContent,
    ).toBe("8");
    expect(container.querySelector(".trace-code-keyword")).not.toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(container.textContent).toContain("<script>");
    expect(container.querySelectorAll(".trace-diff-file")).toHaveLength(2);
    expect(
      container.querySelectorAll(".trace-diff-file")[1].textContent,
    ).toContain("40");
  });

  it("separates opening a file from expanding its historical execution detail", async () => {
    const open = vi.fn();
    await act(async () =>
      root.render(
        <AwakenToolTraceLine
          title="读取 a.ts"
          fileLabel="a.ts"
          onOpenFile={open}
          output="旧文件内容"
        />,
      ),
    );
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>(".tool-trace-file-link")!
        .click(),
    );
    expect(open).toHaveBeenCalledOnce();
    expect(container.querySelector(".tool-trace-output-card")).toBeNull();
    await act(async () =>
      container
        .querySelector<HTMLButtonElement>(".tool-trace-detail-toggle")!
        .click(),
    );
    expect(container.textContent).toContain("旧文件内容");
    expect(open).toHaveBeenCalledOnce();
    expect(container.querySelector("button button")).toBeNull();
  });

  it("only resolves registered project files and never guesses same-name targets", () => {
    const folders = [
      { path: "/work/project", isPrimary: true },
      { path: "/work/extra" },
    ];
    expect(traceFileTarget("p", folders, "src/a.ts", 8)).toMatchObject({
      projectId: "p",
      folderPath: "/work/project",
      relativePath: "src/a.ts",
      lineRequest: { startLine: 8 },
    });
    expect(traceFileTarget("p", folders, "/work/extra/b.ts")).toMatchObject({
      folderPath: "/work/extra",
      relativePath: "b.ts",
    });
    for (const path of [
      "../a.ts",
      "/work/project-other/a.ts",
      "/work/project/../private",
      "/private/a.ts",
    ])
      expect(traceFileTarget("p", folders, path)).toBeUndefined();
    expect(traceFileTarget(undefined, folders, "a.ts")).toBeUndefined();
  });

  it("keeps identical root and child tool IDs in separate detail records", () => {
    let state = createInitialRuntimeState();
    for (const task of [undefined, "child-1", "child-2"]) {
      const common = {
        runId: "r",
        toolCallId: "same",
        toolCallName: "Read",
        subagentTaskId: task,
      };
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: { ...common, type: "TOOL_CALL_START" },
      });
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: {
          ...common,
          type: "TOOL_CALL_ARGS",
          delta: JSON.stringify({ file_path: `${task ?? "root"}.ts` }),
        },
      });
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: {
          ...common,
          type: "TOOL_CALL_RESULT",
          toolStatus: "succeeded",
          content: `${task ?? "root"} result`,
        },
      });
    }
    expect(state.toolResults[0].content).toBe("root result");
    expect(state.subagentsById["child-1"].toolResults?.[0]).toMatchObject({
      id: "child-1:same",
      runId: "r:subagent:child-1",
      content: "child-1 result",
    });
    expect(state.subagentsById["child-2"].toolResults?.[0].args).toContain(
      "child-2.ts",
    );
  });

  it("retains scoped history across runs, reconciles duplicate results and invalidates rewound suffixes", () => {
    let state = createInitialRuntimeState();
    for (const runId of ["one", "two"]) {
      state = reduceRuntimeState(state, {
        type: "run/requested",
        prompt: runId,
        runId,
      });
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: { type: "RUN_STARTED", threadId: "thread", runId },
      });
      const event = { runId, toolCallId: "same", toolCallName: "Read" };
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: { ...event, type: "TOOL_CALL_START" },
      });
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: {
          ...event,
          type: "TOOL_CALL_ARGS",
          delta: JSON.stringify({ file_path: `${runId}.ts` }),
        },
      });
      for (let i = 0; i < 2; i++)
        state = reduceRuntimeState(state, {
          type: "run/event",
          event: {
            ...event,
            type: "TOOL_CALL_RESULT",
            content: `${runId} result`,
            toolStatus: "succeeded",
          },
        });
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: { ...event, type: "TOOL_PROGRESS", progress: "late progress" },
      });
      state = reduceRuntimeState(state, {
        type: "run/event",
        event: { type: "RUN_FINISHED", runId },
      });
    }
    expect(
      state.toolResults.map((record) => [
        record.runId,
        record.content,
        record.status,
      ]),
    ).toEqual([
      ["one", "one result", "done"],
      ["two", "two result", "done"],
    ]);
    state = reduceRuntimeState(state, {
      type: "run/promptReplaced",
      targetRunId: "two",
      prompt: "replacement",
    });
    expect(state.toolResults.map((record) => record.runId)).toEqual(["one"]);
    state = reduceRuntimeState(state, { type: "run/reset" });
    expect(state.toolResults).toEqual([]);
  });
});
