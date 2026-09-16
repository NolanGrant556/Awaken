import {
  AwakenRunStage,
  AwakenWorkbenchChrome,
  AwakenWorkbenchShell,
} from "@awaken/design-system";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { readStyleSource } from "./style-test-source";
import {
  ExpandedFileComposer,
  expandedFileComposerCompletionKey,
  expandedFileComposerPresentation,
  formatExpandedFileComposerDuration,
  shouldCollapseExpandedFileComposer,
} from "./ExpandedFileComposer";
import {
  shouldRenderWorkbenchNarrative,
  shouldShowRunPlanSurface,
} from "./WorkbenchSurfacePolicy";

describe("expanded file workspace shell", () => {
  it("lets expanded preview clicks pass through empty composer stack without moving the dock", () => {
    const workbenchStyles = readStyleSource(
      new URL("./styles/workbench.css", import.meta.url),
    );
    const projectFileStyles = readStyleSource(
      new URL("./styles/project-files.css", import.meta.url),
    );
    const shellStyles = readStyleSource(
      new URL(
        "../../../../Awaken Design System/src/components/AwakenWorkbenchShell/AwakenWorkbenchShell.css",
        import.meta.url,
      ),
    );

    expect(workbenchStyles).toMatch(
      /\.aw-run-stage > \.aw-composer-dock > \.composer-dock-stack\s*\{\s*pointer-events:\s*none;/,
    );
    expect(projectFileStyles).toMatch(
      /\.expanded-file-composer\s*\{[^}]*pointer-events:\s*none;/,
    );
    expect(projectFileStyles).toMatch(
      /\.expanded-file-composer__full,\s*\.expanded-file-composer__rail,\s*\.expanded-file-composer__pill\s*\{\s*pointer-events:\s*auto;/,
    );
    expect(shellStyles).not.toContain(
      ".aw-workbench-shell-inspector-expanded .aw-run-stage > .aw-composer-dock",
    );
  });

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
        idleMode: "compact",
      }),
    ).toBe("running");
    expect(
      expandedFileComposerPresentation({
        active: true,
        forceFull: false,
        running: true,
        completed: false,
        idleMode: "full",
      }),
    ).toBe("full");
    expect(
      expandedFileComposerPresentation({
        active: true,
        forceFull: false,
        running: true,
        completed: false,
        idleMode: "rail",
      }),
    ).toBe("rail");
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

  it("keeps stop on a dedicated button in the running pill", () => {
    const html = renderToString(
      <ExpandedFileComposer
        active
        idleMode="compact"
        running
        completed={false}
        onShowCompact={() => undefined}
        onShowFull={() => undefined}
        onStop={() => undefined}
      >
        <div>原输入框</div>
      </ExpandedFileComposer>,
    );

    expect(html).toContain('data-expanded-file-composer-state="running"');
    expect(html).toContain("正在执行");
    expect(html).toContain("停止运行");
    expect(html).not.toContain("点击停止");
    expect(html).not.toContain("原输入框");
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

  it("collapses the active surface independently of draft contents and protects required panels", () => {
    expect(shouldCollapseExpandedFileComposer({ active: true, forceFull: false })).toBe(true);
    expect(shouldCollapseExpandedFileComposer({ active: false, forceFull: false })).toBe(false);
    expect(shouldCollapseExpandedFileComposer({ active: true, forceFull: true })).toBe(false);
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
