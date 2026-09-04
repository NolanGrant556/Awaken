import React from "react";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import {
  AwakenRunStage,
  AwakenWorkbenchChrome,
  AwakenWorkbenchShell,
} from "@awaken/design-system";
import {
  ExpandedFileComposer,
  expandedFileComposerCompletionKey,
  expandedFileComposerPresentation,
  formatExpandedFileComposerDuration,
  hasExpandedFileComposerDraft,
  shouldCollapseExpandedFileComposer,
} from "./ExpandedFileComposer";
import {
  shouldRenderWorkbenchNarrative,
  shouldShowRunPlanSurface,
} from "./WorkbenchSurfacePolicy";

describe("expanded file workspace shell", () => {
  it("gives the expanded file surface the shell while keeping the composer mounted", () => {
    const stage = (
      <AwakenRunStage contentInactive composer={<div>继续输入</div>}>
        <div>会话叙事</div>
      </AwakenRunStage>
    );
    const html = renderToString(
      <AwakenWorkbenchShell
        stage={stage}
        inspector={<div>文件工作区</div>}
        inspectorExpanded
        inspectorSurfaceActive
        inspectorResizeHandleProps={{ "aria-label": "Resize inspector" }}
      />,
    );

    expect(html).toContain("aw-workbench-shell-inspector-expanded");
    expect(html).toContain("aw-run-stage-scroll-inactive");
    expect(html).toContain("继续输入");
    expect(html).toContain("文件工作区");
    expect(html).not.toContain("aw-workbench-inspector-resize-handle");
  });

  it("hides but does not remove the file surface while the conversation tab is active", () => {
    const html = renderToString(
      <AwakenWorkbenchShell
        stage={<div>当前会话</div>}
        inspector={<div>README.md</div>}
        inspectorExpanded
        inspectorSurfaceActive={false}
      />,
    );

    expect(html).toContain("aw-workbench-inspector-surface-hidden");
    expect(html).toContain('aria-hidden="true"');
    expect(html).toContain("README.md");
  });

  it("renders independent center and right chrome slots for conversation and file tabs", () => {
    const html = renderToString(
      <AwakenWorkbenchChrome
        platform="macos"
        center={<button>当前会话</button>}
        right={<button>README.md</button>}
      />,
    );

    expect(html).toContain("workbench-chrome-center");
    expect(html).toContain("当前会话");
    expect(html).toContain("README.md");
  });

  it("projects the expanded file composer without creating a third ComposerBar variant", () => {
    expect(
      expandedFileComposerPresentation({
        active: false,
        forceFull: false,
        running: false,
        completed: false,
        idleMode: "rail",
      }),
    ).toBe("full");
    expect(
      expandedFileComposerPresentation({
        active: true,
        forceFull: false,
        running: false,
        completed: false,
        idleMode: "rail",
      }),
    ).toBe("rail");
    expect(
      expandedFileComposerPresentation({
        active: true,
        forceFull: false,
        running: true,
        completed: false,
        idleMode: "full",
      }),
    ).toBe("running");
    expect(
      expandedFileComposerPresentation({
        active: true,
        forceFull: false,
        running: false,
        completed: true,
        idleMode: "full",
      }),
    ).toBe("completed");
    expect(
      expandedFileComposerPresentation({
        active: true,
        forceFull: true,
        running: true,
        completed: false,
        idleMode: "rail",
      }),
    ).toBe("full");
  });

  it("unmounts the expensive full composer behind compact presentations", () => {
    const html = renderToString(
      <ExpandedFileComposer
        active
        idleMode="compact"
        running={false}
        completed={false}
        onShowCompact={() => undefined}
        onShowFull={() => undefined}
      >
        <div>原输入框</div>
      </ExpandedFileComposer>,
    );

    expect(html).toContain('data-expanded-file-composer-state="compact"');
    expect(html).not.toContain("原输入框");
    expect(html).toContain("继续对话");
  });

  it("suspends narrative and run-plan surfaces while the file surface is active", () => {
    expect(
      shouldRenderWorkbenchNarrative({
        showCenterComposer: false,
        fileWorkspaceSurfaceActive: true,
      }),
    ).toBe(false);
    expect(
      shouldRenderWorkbenchNarrative({
        showCenterComposer: false,
        fileWorkspaceSurfaceActive: false,
      }),
    ).toBe(true);
    expect(
      shouldShowRunPlanSurface({
        fileWorkspaceSurfaceActive: true,
        mediaPreviewActive: false,
      }),
    ).toBe(false);
  });

  it("formats the completed status in Chinese", () => {
    expect(formatExpandedFileComposerDuration()).toBe("已完成");
    expect(formatExpandedFileComposerDuration(500)).toBe("已完成 · 1 秒");
    expect(formatExpandedFileComposerDuration(65_000)).toBe(
      "已完成 · 1 分 5 秒",
    );
  });

  it("only collapses an idle empty composer on the active file surface", () => {
    expect(
      hasExpandedFileComposerDraft({
        effectiveText: "   ",
        attachmentCount: 0,
        tokenCount: 0,
      }),
    ).toBe(false);
    expect(
      hasExpandedFileComposerDraft({
        effectiveText: "正文",
        attachmentCount: 0,
        tokenCount: 0,
      }),
    ).toBe(true);
    expect(
      hasExpandedFileComposerDraft({
        effectiveText: "",
        attachmentCount: 1,
        tokenCount: 0,
      }),
    ).toBe(true);
    expect(
      hasExpandedFileComposerDraft({
        effectiveText: "",
        attachmentCount: 0,
        tokenCount: 1,
      }),
    ).toBe(true);
    expect(
      shouldCollapseExpandedFileComposer({
        active: true,
        busy: false,
        forceFull: false,
        hasDraft: false,
      }),
    ).toBe(true);
    for (const protectedState of [
      { active: false, busy: false, forceFull: false, hasDraft: false },
      { active: true, busy: true, forceFull: false, hasDraft: false },
      { active: true, busy: false, forceFull: true, hasDraft: false },
      { active: true, busy: false, forceFull: false, hasDraft: true },
    ]) {
      expect(shouldCollapseExpandedFileComposer(protectedState)).toBe(false);
    }
  });

  it("shows completion only for the run submitted from the expanded file composer", () => {
    expect(
      expandedFileComposerCompletionKey({
        status: "finished",
        currentThreadId: "thread-old",
        currentRunId: "run-old",
        completedAtMs: 100,
      }),
    ).toBeUndefined();
    expect(
      expandedFileComposerCompletionKey({
        trackedRun: { threadId: "thread-file", runId: "run-file" },
        status: "finished",
        currentThreadId: "thread-old",
        currentRunId: "run-old",
        completedAtMs: 100,
      }),
    ).toBeUndefined();
    expect(
      expandedFileComposerCompletionKey({
        trackedRun: { threadId: "thread-file", runId: "run-file" },
        status: "finished",
        currentThreadId: "thread-file",
        currentRunId: "run-file",
        completedAtMs: 100,
      }),
    ).toBe("thread-file:run-file:100");
  });
});
