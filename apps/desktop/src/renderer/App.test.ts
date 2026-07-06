import React from "react";
import {
  AwakenComposerBar,
  AwakenProjectTree,
  AwakenUserPromptBlock,
  pastedImageFilesFromClipboardData,
  shouldSubmitComposerInputKey,
} from "@awaken/design-system";
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  App,
  buildWorkspaceRegistryPersistRequest,
  resolveWorkspaceRootForRun,
  shouldResetActiveThreadForWorkspaceChange,
  shouldShowSidebarThreadSpinner,
} from "./App";
import { composerRuntimePanelsForStatus } from "./useComposerLifecycle";
import type { RunNarrativeItem } from "./run-narrative";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  assignThreadToProject,
  mergeWorkspaceProjects,
  normalizeWorkspacePath,
  projectIdForThread,
  removeThreadProjectAssignments,
  removeWorkspaceProject,
  upsertWorkspaceProject,
  workspaceLabelForPath,
} from "./projectWorkspace";
import {
  anchoredTurnMinHeight,
  activeTurnIdForAnchor,
  composerBottomSafeArea,
  composerContentBottomInset,
  diffLineStats,
  groupNarrativeByTurns,
  isNearLatestTurnOutputTail,
  isNearScrollBottom,
  latestTurnMinHeightBottomReserve,
  latestTurnPromptAnchorSlack,
  projectNarrativeTurnBodySegments,
  resolvedDecisionForApproval,
  scrollTopForLatestTurnScrollTail,
  scrollTopForPromptAnchor,
  shouldCancelPendingPromptAnchorForScrollIntent,
  shouldFollowLatestTurnScrollTail,
  shouldPauseLatestOutputFollowForScroll,
  shouldPauseLatestOutputFollowForTouchMove,
  shouldPauseLatestOutputFollowForWheel,
  shouldRequestLatestOutputFollowResumeForTouchMove,
  shouldRequestLatestOutputFollowResumeForWheel,
  shouldResumeLatestOutputFollow,
  shouldShowAssistantAnswerFooter,
  shouldShowReturnToLatestAffordance,
  shouldShowTurnMarkerRail,
  shouldUseCenterComposer,
  turnMarkerRailDisplay,
} from "./workbenchPresentation";
import type { AwakenBridge } from "../shared/runtime";

function stubElectronBridge(): void {
  vi.stubGlobal("window", {
    awaken: {
      platform: "macos",
      getRun: vi.fn(),
      getThreadTranscript: vi.fn(),
      listThreads: vi.fn(async () => []),
      renameThread: vi.fn(),
      deleteThread: vi.fn(),
      startRun: vi.fn(),
      cancelRun: vi.fn(),
      resolveApproval: vi.fn(),
      subscribeRun: vi.fn(),
      unsubscribeRun: vi.fn(),
      getRuntimeInfo: vi.fn(),
      listConnectors: vi.fn(async () => []),
      connectConnector: vi.fn(),
      disconnectConnector: vi.fn(),
      listMcpServers: vi.fn(async () => []),
      configureMcpServers: vi.fn(),
      getMcpServerConfig: vi.fn(),
      updateMcpServerConfig: vi.fn(),
      disableMcpServer: vi.fn(),
      enableMcpServer: vi.fn(),
      deleteMcpServer: vi.fn(),
      getRuntimeLogs: vi.fn(async () => []),
      listMcpTools: vi.fn(async () => []),
      listSkills: vi.fn(async () => []),
      installSkill: vi.fn(),
      listModelProviders: vi.fn(async () => []),
      listModelProfiles: vi.fn(async () => []),
      saveModelProfile: vi.fn(),
      deleteModelProfile: vi.fn(),
      setDefaultModelProfile: vi.fn(),
      testModelProfile: vi.fn(async () => ({ ok: true, message: "ok" })),
      pickWorkspace: vi.fn(async () => ({ canceled: true as const })),
      validateWorkspace: vi.fn(async () => ({ valid: true as const })),
      persistWorkspaceRegistry: vi.fn(async () => undefined),
      getSupervisorStatus: vi.fn(async () => ({ status: "running" as const })),
      onSupervisorStatusChange: vi.fn(() => () => {}),
      notifyRendererReady: vi.fn(),
    } satisfies AwakenBridge,
  });
}

describe("resolvedDecisionForApproval", () => {
  it("returns undefined when both approval states are empty", () => {
    expect(resolvedDecisionForApproval(undefined, null)).toBeUndefined();
  });

  it("returns undefined when the resolved approval belongs to another approval", () => {
    expect(
      resolvedDecisionForApproval(
        { id: "approval-1" },
        { approvalId: "approval-2", decision: "approved" },
      ),
    ).toBeUndefined();
  });

  it("returns the decision for the active approval", () => {
    expect(
      resolvedDecisionForApproval(
        { id: "approval-1" },
        { approvalId: "approval-1", decision: "rejected" },
      ),
    ).toBe("rejected");
  });
});

describe("shouldResetActiveThreadForWorkspaceChange", () => {
  it("returns false when no active thread workspace exists", () => {
    expect(
      shouldResetActiveThreadForWorkspaceChange(undefined, "/tmp/project"),
    ).toBe(false);
  });

  it("returns false when switching within the same workspace", () => {
    expect(
      shouldResetActiveThreadForWorkspaceChange(
        "/tmp/project",
        "/tmp/project",
      ),
    ).toBe(false);
  });

  it("returns true when switching to a different workspace", () => {
    expect(
      shouldResetActiveThreadForWorkspaceChange(
        "/tmp/project-a",
        "/tmp/project-b",
      ),
    ).toBe(true);
  });
});

describe("resolveWorkspaceRootForRun", () => {
  it("prefers the immediate workspace target over stale workspace state", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/old",
        workspaceTargetPath: "/tmp/new",
        selectedProjectId: "workspace:%2Ftmp%2Fnew",
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [
          {
            id: "workspace:%2Ftmp%2Fnew",
            label: "new",
            path: "/tmp/new",
          },
        ],
      }),
    ).toBe("/tmp/new");
  });

  it("uses the selected project path before a stale workspace path", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/old",
        selectedProjectId: "workspace:%2Ftmp%2Fknowledge",
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [
          {
            id: "workspace:%2Ftmp%2Fknowledge",
            label: "knowledge",
            path: "/tmp/knowledge",
          },
        ],
      }),
    ).toBe("/tmp/knowledge");
  });

  it("uses the default workspace when the built-in project is selected", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "/tmp/old",
        selectedProjectId: DEFAULT_WORKSPACE_PROJECT_ID,
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [],
      }),
    ).toBe("/tmp/default");
  });

  it("does not fall back to the hidden default workspace after that project is removed", () => {
    expect(
      resolveWorkspaceRootForRun({
        workspacePath: "",
        selectedProjectId: undefined,
        defaultWorkspacePath: "/tmp/default",
        defaultProjectRemoved: true,
        projectsFromWorkspace: [],
      }),
    ).toBe("");
  });
});

describe("buildWorkspaceRegistryPersistRequest", () => {
  it("omits the default workspace after the default project is removed", () => {
    expect(
      buildWorkspaceRegistryPersistRequest({
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [],
        threadWorkspacePaths: [],
        defaultProjectRemoved: true,
      }),
    ).toEqual({
      defaultWorkspacePath: "",
      registeredWorkspacePaths: [],
    });
  });

  it("keeps non-default visible projects and promotes them to default when needed", () => {
    expect(
      buildWorkspaceRegistryPersistRequest({
        defaultWorkspacePath: "/tmp/default",
        pathForSelectedProject: "/tmp/knowledge",
        projectsFromWorkspace: [
          {
            id: "workspace:%2Ftmp%2Fknowledge",
            label: "knowledge",
            path: "/tmp/knowledge",
          },
        ],
        threadWorkspacePaths: [],
        defaultProjectRemoved: true,
      }),
    ).toEqual({
      defaultWorkspacePath: "/tmp/knowledge",
      registeredWorkspacePaths: ["/tmp/knowledge"],
    });
  });

  it("retains the default workspace while threads still belong to it", () => {
    expect(
      buildWorkspaceRegistryPersistRequest({
        defaultWorkspacePath: "/tmp/default",
        projectsFromWorkspace: [],
        threadWorkspacePaths: ["/tmp/default"],
        defaultProjectRemoved: true,
      }),
    ).toEqual({
      defaultWorkspacePath: "/tmp/default",
      registeredWorkspacePaths: ["/tmp/default"],
    });
  });
});

describe("AwakenComposerInput keyboard behavior", () => {
  it("submits with Enter and keeps Shift+Enter for new lines", () => {
    expect(shouldSubmitComposerInputKey({ key: "Enter" })).toBe(true);
    expect(shouldSubmitComposerInputKey({ key: "Enter", shiftKey: true })).toBe(
      false,
    );
    expect(
      shouldSubmitComposerInputKey({ key: "Enter", isComposing: true }),
    ).toBe(false);
    expect(shouldSubmitComposerInputKey({ key: "a" })).toBe(false);
  });

  it("collects pasted image files without pulling in non-image clipboard files", () => {
    const imageFile = new File(["image"], "paste.png", { type: "image/png" });
    const textFile = new File(["text"], "notes.txt", { type: "text/plain" });

    const files = pastedImageFilesFromClipboardData({
      items: [] as unknown as DataTransferItemList,
      files: [imageFile, textFile] as unknown as FileList,
    });

    expect(files).toHaveLength(1);
    expect(files[0]?.name).toBe("paste.png");
  });
});

describe("composer lifecycle presentation", () => {
  it("builds runtime panels without changing their placement order", () => {
    expect(
      composerRuntimePanelsForStatus({
        desktopNeedsRestart: true,
        desktopStarting: true,
        runtimeErrorMessage: "boom",
      }),
    ).toEqual([
      {
        id: "restart",
        title: "Awaken needs restart",
        message: "Quit Awaken and open it again.",
        tone: "error",
      },
      {
        id: "starting",
        title: "Starting Awaken",
        message: "Preparing the desktop app.",
      },
      {
        id: "run-error",
        title: "Run error",
        message: "boom",
        tone: "error",
      },
    ]);
  });
});

describe("assistant answer footer contract", () => {
  it("shows final answer actions only for completed final answers", () => {
    expect(
      shouldShowAssistantAnswerFooter({ status: "done", isFinal: true }),
    ).toBe(true);
    expect(shouldShowAssistantAnswerFooter({ status: "done" })).toBe(false);
    expect(
      shouldShowAssistantAnswerFooter({ status: "active", isFinal: true }),
    ).toBe(false);
    expect(
      shouldShowAssistantAnswerFooter({ status: "error", isFinal: true }),
    ).toBe(false);
  });
});

describe("AwakenComposerBar layout contract", () => {
  it("keeps dock input at one row while center keeps the homepage row budget", () => {
    const baseProps = {
      value: "",
      onChange: vi.fn(),
      onSubmit: vi.fn(),
    };

    const dockHtml = renderToString(
      React.createElement(AwakenComposerBar, { ...baseProps, variant: "dock" }),
    );
    const centerHtml = renderToString(
      React.createElement(AwakenComposerBar, {
        ...baseProps,
        variant: "center",
      }),
    );

    expect(dockHtml).toContain("composer-bar-dock");
    expect(dockHtml).toContain('rows="1"');
    expect(centerHtml).toContain("composer-bar-center");
    expect(centerHtml).toContain('rows="3"');
  });

  it("renders attachment previews and warning copy inside the composer surface", () => {
    const html = renderToString(
      React.createElement(AwakenComposerBar, {
        value: "",
        onChange: vi.fn(),
        onSubmit: vi.fn(),
        variant: "dock",
        attachments: [
          {
            id: "image-1",
            kind: "image",
            src: "data:image/png;base64,aGVsbG8=",
            alt: "clip.png",
            mimeType: "image/png",
          },
        ],
        attachmentWarning: "当前模型不支持图片输入，请切换模型或移除图片",
      }),
    );

    expect(html).toContain("composer-bar-attachments");
    expect(html).toContain("clip.png");
    expect(html).toContain("composer-bar-attachment-warning");
    expect(html).toContain("当前模型不支持图片输入");
  });
});

describe("AwakenUserPromptBlock layout contract", () => {
  it("renders the prompt bubble as a collapsed expandable surface", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "1. First line\n2. Second line\n3. Third line\n4. Fourth line",
      }),
    );

    expect(html).toContain("execution-prompt-block-limited");
    expect(html).toContain("execution-prompt-bubble");
    expect(html).toContain("execution-prompt-bubble-text");
    expect(html).toContain("<p");
    expect(html).toContain('role="button"');
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain("4. Fourth line");
  });

  it("keeps short prompt bubbles non-expandable", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "One short prompt",
      }),
    );

    expect(html).toContain("execution-prompt-bubble");
    expect(html).toContain("execution-prompt-bubble-text");
    expect(html).toContain("<p");
    expect(html).not.toContain("execution-prompt-block-limited");
    expect(html).toContain('role="button"');
    expect(html).toContain('aria-expanded="false"');
  });

  it("allows the expanded state to be controlled by the parent renderer", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "1. First line\n2. Second line\n3. Third line\n4. Fourth line",
        expanded: true,
      }),
    );

    expect(html).toContain("execution-prompt-block-expanded");
    expect(html).not.toContain('aria-expanded="false"');
  });

  it("renders prompt attachments above the prompt text", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "Look at this image",
        attachments: [
          {
            id: "attachment-1",
            src: "data:image/png;base64,aGVsbG8=",
            alt: "Prompt image",
          },
        ],
      }),
    );

    expect(html).toContain("execution-prompt-bubble-attachments");
    expect(html).toContain("execution-prompt-bubble-attachment");
    expect(html).toContain("Prompt image");
    expect(html).toContain('role="button"');
  });

  it("shows the prompt toolbar for short prompts once expanded", () => {
    const html = renderToString(
      React.createElement(AwakenUserPromptBlock, {
        text: "One short prompt",
        expanded: true,
      }),
    );

    expect(html).toContain("execution-prompt-bubble-toolbar");
    expect(html).toContain("Copy prompt");
  });
});

describe("project workspace helpers", () => {
  it("normalizes workspace paths and labels them by folder name", () => {
    expect(normalizeWorkspacePath("/Users/nolangrant/Awaken/")).toBe(
      "/Users/nolangrant/Awaken",
    );
    expect(workspaceLabelForPath("/Users/nolangrant/Awaken/")).toBe("Awaken");
  });

  it("maps the default workspace to the built-in project", () => {
    const result = upsertWorkspaceProject(
      [],
      "/Users/nolangrant/Awaken",
      "/Users/nolangrant/Awaken/",
    );

    expect(result.projectId).toBe(DEFAULT_WORKSPACE_PROJECT_ID);
    expect(result.projects).toEqual([]);
  });

  it("adds and deduplicates local folder projects", () => {
    const first = upsertWorkspaceProject(
      [],
      "/Users/nolangrant/Other",
      "/Users/nolangrant/Awaken",
    );
    const second = upsertWorkspaceProject(
      first.projects,
      "/Users/nolangrant/Other/",
      "/Users/nolangrant/Awaken",
    );

    expect(first.projectId).toBe(second.projectId);
    expect(second.projects).toHaveLength(1);
    expect(second.projects[0]).toMatchObject({
      label: "Other",
      path: "/Users/nolangrant/Other",
    });
  });

  it("assigns new threads to the selected project with default fallback", () => {
    const mapping = assignThreadToProject(
      {},
      "thread-1",
      "workspace:%2Ftmp%2Fdemo",
    );

    expect(projectIdForThread("thread-1", mapping)).toBe(
      "workspace:%2Ftmp%2Fdemo",
    );
    expect(projectIdForThread("thread-2", mapping)).toBe(
      DEFAULT_WORKSPACE_PROJECT_ID,
    );
  });

  it("prefers a thread workspace root over the transient project mapping", () => {
    const mapping = assignThreadToProject(
      {},
      "thread-1",
      "workspace:%2Ftmp%2Fold",
    );

    expect(
      projectIdForThread(
        { threadId: "thread-1", workspaceRoot: "/tmp/demo" },
        mapping,
        "/tmp/default",
      ),
    ).toBe("workspace:%2Ftmp%2Fdemo");
    expect(
      projectIdForThread(
        { threadId: "thread-1", workspaceRoot: "/tmp/default" },
        mapping,
        "/tmp/default",
      ),
    ).toBe(DEFAULT_WORKSPACE_PROJECT_ID);
  });

  it("removes folder projects and thread assignments", () => {
    const projects = [
      { id: "workspace:%2Ftmp%2Fdemo", label: "demo", path: "/tmp/demo" },
      { id: "workspace:%2Ftmp%2Fkeep", label: "keep", path: "/tmp/keep" },
    ];
    const mapping = {
      "thread-1": "workspace:%2Ftmp%2Fdemo",
      "thread-2": "workspace:%2Ftmp%2Fkeep",
    };

    expect(removeWorkspaceProject(projects, "workspace:%2Ftmp%2Fdemo")).toEqual(
      [{ id: "workspace:%2Ftmp%2Fkeep", label: "keep", path: "/tmp/keep" }],
    );
    expect(removeThreadProjectAssignments(mapping, ["thread-1"])).toEqual({
      "thread-2": "workspace:%2Ftmp%2Fkeep",
    });
  });

  it("merges runtime thread workspaces into the persisted project list", () => {
    expect(
      mergeWorkspaceProjects(
        [
          {
            id: "workspace:%2Ftmp%2Fold",
            label: "old",
            path: "/tmp/old",
          },
        ],
        ["/tmp/old", "/tmp/demo", "/tmp/default"],
        "/tmp/default",
      ),
    ).toEqual([
      { id: "workspace:%2Ftmp%2Fold", label: "old", path: "/tmp/old" },
      { id: "workspace:%2Ftmp%2Fdemo", label: "demo", path: "/tmp/demo" },
    ]);
  });
});

describe("sidebar thread status presentation", () => {
  it("shows a spinner only while the thread still has an active run", () => {
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-running",
        label: "Running",
        runStatus: "running",
        activeRunId: "run-1",
      }),
    ).toBe(true);
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-awaiting",
        label: "Awaiting approval",
        runStatus: "awaiting_approval",
        activeRunId: "run-2",
      }),
    ).toBe(false);
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-recovering",
        label: "Recovering",
        activeRunId: "run-3",
      }),
    ).toBe(true);
    expect(
      shouldShowSidebarThreadSpinner({
        threadId: "thread-done",
        label: "Done",
        runStatus: "finished",
        activeRunId: undefined,
      }),
    ).toBe(false);
  });

  it("keeps the loading spinner visible even for the selected thread row", () => {
    const html = renderToString(
      React.createElement(AwakenProjectTree, {
        items: [
          {
            id: "project:awaken",
            label: "Awaken",
            kind: "project",
            children: [
              {
                id: "thread:active",
                kind: "thread",
                label: "正在执行的会话",
                isLoading: true,
              },
            ],
          },
        ],
        activeId: "thread:active",
        defaultExpandedIds: ["project:awaken"],
        onSelect: vi.fn(),
      }),
    );

    expect(html).toContain("正在执行的会话");
    expect(html).toContain('aria-current="true"');
    expect(html).toContain('aria-label="加载中"');
  });
});

describe("App workbench presentation", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("renders idle as a blank execution stage with the light shell", () => {
    stubElectronBridge();

    const html = renderToString(React.createElement(App));

    expect(html).toContain("On Awaken");
    expect(html).toContain("aw-task-welcome__logo");
    expect(html).toContain("把任务交给 Awaken，自主执行，唤醒自由。");
    expect(html).toContain('placeholder=""');
    expect(html).toContain('rows="3"');
    expect(html).toContain("New Agent");
    expect(html).not.toContain("⌘N");
    expect(html).toContain("Add folder");
    expect(html).toContain("连接器");
    expect(html).toContain("技能");
    expect(html).toContain("请点击头像配置模型");
    expect(html).not.toContain(">Models<");
    expect(html).toContain("composer-bar-center");
    expect(html).not.toContain("Plan, Build, / for skills, @ for context");
    expect(html).not.toContain("composer-bar-dock");
    expect(html).not.toContain("Planning next moves");
    expect(html).not.toContain("Ready for a run");
    expect(html).not.toContain("No narrative yet");
    expect(html).not.toContain("等待运行");
    expect(html).not.toContain("run-stage-header");
    expect(html).not.toContain("run-timeline-shell");
    expect(html).toContain("composer-runtime-panels");
    expect(html).toContain("Starting Awaken");
  });

  it("uses the dock composer only after a run exists or narrative appears", () => {
    expect(
      shouldUseCenterComposer({
        hasNarrative: false,
        hasRun: false,
        hasNotice: false,
        hasError: false,
        needsRestart: false,
      }),
    ).toBe(true);

    expect(
      shouldUseCenterComposer({
        hasNarrative: true,
        hasRun: false,
        hasNotice: false,
        hasError: false,
        needsRestart: false,
      }),
    ).toBe(false);

    expect(
      shouldUseCenterComposer({
        hasNarrative: false,
        hasRun: true,
        hasNotice: false,
        hasError: false,
        needsRestart: false,
      }),
    ).toBe(false);

    expect(
      shouldUseCenterComposer({
        hasNarrative: false,
        hasRun: false,
        hasNotice: true,
        hasError: true,
        needsRestart: true,
      }),
    ).toBe(true);
  });

  it("shows turn markers once the run stage has at least three turns", () => {
    expect(
      shouldShowTurnMarkerRail({
        showCenterComposer: true,
        turnCount: 4,
      }),
    ).toBe(false);
    expect(
      shouldShowTurnMarkerRail({
        showCenterComposer: false,
        turnCount: 2,
      }),
    ).toBe(false);
    expect(
      shouldShowTurnMarkerRail({
        showCenterComposer: false,
        turnCount: 3,
      }),
    ).toBe(true);
  });

  it("adapts turn marker density to the available rail height", () => {
    const eightTurns = Array.from(
      { length: 8 },
      (_, index) => `turn-${index + 1}`,
    );
    expect(
      turnMarkerRailDisplay({
        showCenterComposer: false,
        turnIds: eightTurns,
        latestTurnId: "turn-8",
        railHeight: 420,
      }),
    ).toEqual({
      mode: "regular",
      items: eightTurns.map((id, index) => ({ id, index })),
    });

    const twentyTurns = Array.from(
      { length: 20 },
      (_, index) => `turn-${index + 1}`,
    );
    const compactDisplay = turnMarkerRailDisplay({
      showCenterComposer: false,
      turnIds: twentyTurns,
      latestTurnId: "turn-20",
      railHeight: 420,
    });
    expect(compactDisplay.mode).toBe("compact");
    expect(compactDisplay.items).toHaveLength(20);
  });

  it("samples dense turn markers around the active turn without dropping endpoints", () => {
    const fortyTurns = Array.from(
      { length: 40 },
      (_, index) => `turn-${index + 1}`,
    );
    const display = turnMarkerRailDisplay({
      showCenterComposer: false,
      turnIds: fortyTurns,
      activeTurnId: "turn-20",
      latestTurnId: "turn-40",
      railHeight: 110,
    });

    expect(display.mode).toBe("summary");
    expect(display.items.length).toBeLessThanOrEqual(8);
    expect(display.items.map((item) => item.id)).toEqual(
      expect.arrayContaining([
        "turn-1",
        "turn-19",
        "turn-20",
        "turn-21",
        "turn-40",
      ]),
    );
  });

  it("hides dense markers when the rail is too short to render a useful set", () => {
    const turns = Array.from({ length: 40 }, (_, index) => `turn-${index + 1}`);
    expect(
      turnMarkerRailDisplay({
        showCenterComposer: false,
        turnIds: turns,
        latestTurnId: "turn-40",
        railHeight: 20,
      }),
    ).toEqual({ mode: "hidden", items: [] });
  });

  it("counts real diff additions and removals for inspector deltas", () => {
    expect(
      diffLineStats(
        [
          "diff --git a/file b/file",
          "--- a/file",
          "+++ b/file",
          "-old",
          "+new",
          "+another",
          " context",
        ].join("\n"),
      ),
    ).toEqual({ added: 2, removed: 1 });
  });

  it("groups prompt-led narrative items into turn sections", () => {
    const grouped = groupNarrativeByTurns([
      { kind: "thinking", id: "intro", text: "warmup", status: "done" },
      {
        kind: "prompt",
        id: "prompt:thread-1:1",
        text: "你好",
        attachments: [
          {
            id: "image-1",
            kind: "image",
            mimeType: "image/png",
            dataBase64: "aGVsbG8=",
            displayName: "clip.png",
            byteSize: 5,
          },
        ],
      },
      {
        kind: "thinking",
        id: "thinking:1",
        text: "first thought",
        status: "done",
      },
      {
        kind: "compactBoundary",
        id: "compact:thread-1:compact_1",
        status: "finished",
        title: "已自动压缩上下文",
        boundaryId: "compact_1",
      },
      {
        kind: "answer",
        id: "answer:1",
        markdown: "first answer",
        status: "done",
      },
      { kind: "prompt", id: "prompt:thread-1:2", text: "再次告诉我" },
      {
        kind: "answer",
        id: "answer:2",
        markdown: "second answer",
        status: "active",
      },
    ]);

    expect(grouped.leadItems).toEqual([
      { kind: "thinking", id: "intro", text: "warmup", status: "done" },
    ]);
    expect(grouped.turns).toHaveLength(2);
    expect(grouped.turns[0].prompt.text).toBe("你好");
    expect(grouped.turns[0].prompt.attachments).toHaveLength(1);
    expect(grouped.turns[0].items).toEqual([
      {
        kind: "thinking",
        id: "thinking:1",
        text: "first thought",
        status: "done",
      },
      {
        kind: "compactBoundary",
        id: "compact:thread-1:compact_1",
        status: "finished",
        title: "已自动压缩上下文",
        boundaryId: "compact_1",
      },
      {
        kind: "answer",
        id: "answer:1",
        markdown: "first answer",
        status: "done",
      },
    ]);
    expect(grouped.turns[1].prompt.text).toBe("再次告诉我");
    expect(grouped.turns[1].items).toEqual([
      {
        kind: "answer",
        id: "answer:2",
        markdown: "second answer",
        status: "active",
      },
    ]);
  });

  it("keeps the turn body flat until the final answer is marked final", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const tool = {
      kind: "tool",
      id: "tool:1",
      title: "Read file",
      detail: "docs/README.md",
      status: "done",
    } as const;
    const answer = {
      kind: "answer",
      id: "answer:1",
      markdown: "streaming answer",
      status: "active",
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, tool, answer],
        executionProcessOpen: false,
      }),
    ).toEqual([
      {
        kind: "items",
        id: "turn-items:prompt:thread-1:1",
        items: [thinking, tool, answer],
      },
    ]);
  });

  it("collapses all pre-final narrative into an execution-process segment once the final answer exists", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const interimAnswer = {
      kind: "answer",
      id: "answer:1",
      markdown: "我先检查目录结构。",
      status: "done",
    } as const;
    const tool = {
      kind: "tool",
      id: "tool:1",
      title: "Read file",
      detail: "docs/README.md",
      status: "done",
    } as const;
    const finalAnswer = {
      kind: "answer",
      id: "answer:2",
      markdown: "这是最终回答。",
      status: "done",
      isFinal: true,
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, interimAnswer, tool, finalAnswer],
        executionProcessOpen: false,
      }),
    ).toEqual([
      {
        kind: "executionProcess",
        id: "execution-process:prompt:thread-1:1",
        title: "执行过程",
        items: [thinking, interimAnswer, tool],
        open: false,
      },
      {
        kind: "items",
        id: "answer-and-after:prompt:thread-1:1",
        items: [finalAnswer],
      },
    ]);
  });

  it("keeps execution-process items visible when the disclosure is expanded", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const fileChange: Extract<RunNarrativeItem, { kind: "fileChange" }> = {
      kind: "fileChange",
      id: "file:1",
      files: [
        {
          id: "file-entry:1",
          title: "App.tsx",
          status: "done",
        },
      ],
      status: "done",
    };
    const answer = {
      kind: "answer",
      id: "answer:1",
      markdown: "done answer",
      status: "done",
      isFinal: true,
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, fileChange, answer],
        executionProcessOpen: true,
      }),
    ).toEqual([
      {
        kind: "executionProcess",
        id: "execution-process:prompt:thread-1:1",
        title: "执行过程",
        items: [thinking, fileChange],
        open: true,
      },
      {
        kind: "items",
        id: "answer-and-after:prompt:thread-1:1",
        items: [answer],
      },
    ]);
  });

  it("keeps the turn body flat before any answer item exists", () => {
    const thinking = {
      kind: "thinking",
      id: "thinking:1",
      text: "first thought",
      status: "done",
    } as const;
    const approval = {
      kind: "approval",
      id: "approval:1",
      title: "Need approval",
      status: "waiting",
    } as const;

    expect(
      projectNarrativeTurnBodySegments({
        turnId: "prompt:thread-1:1",
        items: [thinking, approval],
        executionProcessOpen: false,
      }),
    ).toEqual([
      {
        kind: "items",
        id: "turn-items:prompt:thread-1:1",
        items: [thinking, approval],
      },
    ]);
  });

  it("calculates the scroll position that pins a prompt at the anchor", () => {
    expect(
      scrollTopForPromptAnchor({
        currentScrollTop: 240,
        containerTop: 100,
        promptTop: 460,
        anchorOffset: 0,
      }),
    ).toBe(600);
    expect(
      scrollTopForPromptAnchor({
        currentScrollTop: 0,
        containerTop: 100,
        promptTop: 50,
        anchorOffset: 0,
      }),
    ).toBe(0);
  });

  it("selects the turn nearest to the prompt anchor for marker state", () => {
    const turns = [
      { id: "turn-1", top: 100 },
      { id: "turn-2", top: 260 },
      { id: "turn-3", top: 480 },
    ];

    expect(activeTurnIdForAnchor({ turns: [], anchorY: 200 })).toBeUndefined();
    expect(activeTurnIdForAnchor({ turns, anchorY: 90 })).toBe("turn-1");
    expect(activeTurnIdForAnchor({ turns, anchorY: 260 })).toBe("turn-2");
    expect(activeTurnIdForAnchor({ turns, anchorY: 999 })).toBe("turn-3");
  });

  it("calculates composer bottom safe area", () => {
    expect(
      composerBottomSafeArea({
        composerHeight: 88,
        safeGap: 24,
      }),
    ).toBe(112);
    expect(
      composerBottomSafeArea({
        composerHeight: -1,
        safeGap: 24,
      }),
    ).toBe(24);
  });

  it("measures composer reserve from real dock content instead of the fade", () => {
    expect(
      composerContentBottomInset({
        dockBottom: 720,
        contentTop: 590,
        fallbackInset: 162,
      }),
    ).toBe(130);
    expect(
      composerContentBottomInset({
        dockBottom: 720,
        fallbackInset: 162,
      }),
    ).toBe(162);
    expect(
      composerContentBottomInset({
        dockBottom: 720,
        contentTop: 760,
        fallbackInset: 162,
      }),
    ).toBe(0);
  });

  it("keeps the latest turn tall enough for sticky prompt reading", () => {
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 0,
      }),
    ).toBe(640);
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 12,
      }),
    ).toBe(628);
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 0,
        bottomReserve: 124,
      }),
    ).toBe(516);
    expect(
      anchoredTurnMinHeight({
        containerHeight: 640,
        anchorOffset: 0,
        bottomReserve: 72,
      }),
    ).toBe(568);
  });

  it("reserves the composer gap for every latest turn height state", () => {
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: false,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(72);
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: true,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(72);
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: false,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(72);
    expect(
      latestTurnMinHeightBottomReserve({
        hasBodyItems: true,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(72);
  });

  it("does not add temporary prompt slack that creates a second bottom target", () => {
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: false,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(0);
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: true,
        isPromptAnchorPending: true,
        outputGap: 72,
      }),
    ).toBe(0);
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: false,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(0);
    expect(
      latestTurnPromptAnchorSlack({
        hasBodyItems: true,
        isPromptAnchorPending: false,
        outputGap: 72,
      }),
    ).toBe(0);
  });

  it("keeps prompt-only latest turns from creating blank scroll bottom", () => {
    const containerHeight = 640;
    const outputGap = 72;
    const latestTurnHeight =
      anchoredTurnMinHeight({
        containerHeight,
        anchorOffset: 0,
        bottomReserve: latestTurnMinHeightBottomReserve({
          hasBodyItems: false,
          isPromptAnchorPending: true,
          outputGap,
        }),
      }) +
      latestTurnPromptAnchorSlack({
        hasBodyItems: false,
        isPromptAnchorPending: true,
        outputGap,
      });

    expect(latestTurnHeight + outputGap).toBe(containerHeight);
  });

  it("uses the same native bottom target for programmatic follow and manual scroll bottom", () => {
    const manualScrollBottom = 620;
    expect(
      scrollTopForLatestTurnScrollTail({
        maxScrollTop: manualScrollBottom,
      }),
    ).toBe(manualScrollBottom);
  });

  it("clamps the latest turn follow target to the native bottom instead of a prompt anchor", () => {
    expect(
      scrollTopForLatestTurnScrollTail({
        maxScrollTop: 320,
      }),
    ).toBe(320);
  });

  it("follows the latest turn tail after prompt anchoring and body output exist", () => {
    expect(
      shouldFollowLatestTurnScrollTail({
        hasBodyItems: false,
        isPromptAnchorPending: false,
      }),
    ).toBe(false);
    expect(
      shouldFollowLatestTurnScrollTail({
        hasBodyItems: true,
        isPromptAnchorPending: true,
      }),
    ).toBe(false);
    expect(
      shouldFollowLatestTurnScrollTail({
        hasBodyItems: true,
        isPromptAnchorPending: false,
      }),
    ).toBe(true);
  });

  it("keeps layout scroll from cancelling the pending prompt anchor", () => {
    expect(
      shouldCancelPendingPromptAnchorForScrollIntent({
        hasDirectUserInput: false,
      }),
    ).toBe(false);
    expect(
      shouldCancelPendingPromptAnchorForScrollIntent({
        hasDirectUserInput: true,
      }),
    ).toBe(true);
  });

  it("detects whether the user is still following the latest output", () => {
    expect(
      isNearLatestTurnOutputTail({
        containerTop: 100,
        outputBottom: 650,
        containerHeight: 640,
        bottomReserve: 120,
        threshold: 48,
      }),
    ).toBe(true);
    expect(
      isNearLatestTurnOutputTail({
        containerTop: 100,
        outputBottom: 700,
        containerHeight: 640,
        bottomReserve: 120,
        threshold: 48,
      }),
    ).toBe(false);
  });

  it("detects whether the native scroll position is at the bottom", () => {
    expect(
      isNearScrollBottom({
        scrollTop: 620,
        scrollHeight: 1260,
        clientHeight: 640,
        threshold: 2,
      }),
    ).toBe(true);
    expect(
      isNearScrollBottom({
        scrollTop: 600,
        scrollHeight: 1260,
        clientHeight: 640,
        threshold: 2,
      }),
    ).toBe(false);
  });

  it("pauses latest output follow as soon as the user scrolls toward history", () => {
    expect(shouldPauseLatestOutputFollowForWheel({ deltaY: -12 })).toBe(true);
    expect(shouldPauseLatestOutputFollowForWheel({ deltaY: 12 })).toBe(false);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: 240,
        nextScrollTop: 220,
      }),
    ).toBe(true);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: 240,
        nextScrollTop: 260,
      }),
    ).toBe(false);
    expect(
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop: null,
        nextScrollTop: 220,
      }),
    ).toBe(false);
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: 12 })).toBe(
      true,
    );
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: -12 })).toBe(
      false,
    );
    expect(
      shouldPauseLatestOutputFollowForTouchMove({
        startY: 100,
        currentY: 112,
      }),
    ).toBe(true);
    expect(
      shouldPauseLatestOutputFollowForTouchMove({
        startY: 100,
        currentY: 96,
      }),
    ).toBe(false);
    expect(
      shouldRequestLatestOutputFollowResumeForTouchMove({
        startY: 100,
        currentY: 88,
      }),
    ).toBe(true);
    expect(
      shouldRequestLatestOutputFollowResumeForTouchMove({
        startY: 100,
        currentY: 112,
      }),
    ).toBe(false);
  });

  it("does not resume latest output follow until the user asks to return to the tail", () => {
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: true,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: false,
      }),
    ).toBe(false);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: true,
        isNearTail: true,
        isAtScrollBottom: false,
        hasResumeIntent: true,
      }),
    ).toBe(false);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: true,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: true,
      }),
    ).toBe(true);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: false,
        isNearTail: true,
        isAtScrollBottom: false,
        hasResumeIntent: false,
      }),
    ).toBe(true);
  });

  it("requires explicit resume intent after an upward wheel pause", () => {
    const pausedByWheel = shouldPauseLatestOutputFollowForWheel({
      deltaY: -18,
    });
    expect(pausedByWheel).toBe(true);

    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: pausedByWheel,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: false,
      }),
    ).toBe(false);

    const resumeIntent = shouldRequestLatestOutputFollowResumeForWheel({
      deltaY: 18,
    });
    expect(resumeIntent).toBe(true);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: pausedByWheel,
        isNearTail: true,
        isAtScrollBottom: true,
        hasResumeIntent: resumeIntent,
      }),
    ).toBe(true);
  });

  it("shows return-to-latest whenever the user paused away from the latest turn", () => {
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "paused_by_user",
        hasLatestTurn: true,
      }),
    ).toBe(true);
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "following_latest",
        hasLatestTurn: true,
      }),
    ).toBe(false);
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "paused_by_user",
        hasLatestTurn: true,
      }),
    ).toBe(true);
    expect(
      shouldShowReturnToLatestAffordance({
        followMode: "paused_by_user",
        hasLatestTurn: false,
      }),
    ).toBe(false);
  });
});
