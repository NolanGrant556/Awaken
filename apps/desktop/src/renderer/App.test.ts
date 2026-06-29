import React from "react";
import { shouldSubmitComposerInputKey } from "@awaken/design-system";
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import {
  DEFAULT_WORKSPACE_PROJECT_ID,
  assignThreadToProject,
  normalizeWorkspacePath,
  projectIdForThread,
  upsertWorkspaceProject,
  workspaceLabelForPath,
} from "./projectWorkspace";
import {
  anchoredTurnMinHeight,
  activeTurnIdForAnchor,
  composerBottomSafeArea,
  diffLineStats,
  groupNarrativeByTurns,
  isNearLatestTurnOutputTail,
  measuredComposerDockHeight,
  resolvedDecisionForApproval,
  scrollTopForLatestTurnOutputTail,
  scrollTopForPromptAnchor,
  shouldPauseLatestOutputFollowForTouchMove,
  shouldPauseLatestOutputFollowForWheel,
  shouldRequestLatestOutputFollowResumeForTouchMove,
  shouldRequestLatestOutputFollowResumeForWheel,
  shouldResumeLatestOutputFollow,
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
      getThreadLite: vi.fn(),
      getThreadContext: vi.fn(),
      getThreadTranscript: vi.fn(),
      getSessionInsights: vi.fn(),
      listThreads: vi.fn(async () => []),
      branchThread: vi.fn(),
      startRun: vi.fn(),
      cancelRun: vi.fn(),
      resolveApproval: vi.fn(),
      subscribeRun: vi.fn(),
      unsubscribeRun: vi.fn(),
      getRuntimeInfo: vi.fn(),
      pickWorkspace: vi.fn(async () => ({ canceled: true as const })),
      validateWorkspace: vi.fn(async () => ({ valid: true as const })),
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

describe("AwakenComposerInput keyboard behavior", () => {
  it("submits with Enter and keeps Shift+Enter for new lines", () => {
    expect(shouldSubmitComposerInputKey({ key: "Enter" })).toBe(true);
    expect(shouldSubmitComposerInputKey({ key: "Enter", shiftKey: true })).toBe(false);
    expect(shouldSubmitComposerInputKey({ key: "Enter", isComposing: true })).toBe(false);
    expect(shouldSubmitComposerInputKey({ key: "a" })).toBe(false);
  });
});

describe("project workspace helpers", () => {
  it("normalizes workspace paths and labels them by folder name", () => {
    expect(normalizeWorkspacePath("/Users/nolangrant/Awaken/")).toBe("/Users/nolangrant/Awaken");
    expect(workspaceLabelForPath("/Users/nolangrant/Awaken/")).toBe("Awaken");
  });

  it("maps the default workspace to the built-in project", () => {
    const result = upsertWorkspaceProject([], "/Users/nolangrant/Awaken", "/Users/nolangrant/Awaken/");

    expect(result.projectId).toBe(DEFAULT_WORKSPACE_PROJECT_ID);
    expect(result.projects).toEqual([]);
  });

  it("adds and deduplicates local folder projects", () => {
    const first = upsertWorkspaceProject([], "/Users/nolangrant/Other", "/Users/nolangrant/Awaken");
    const second = upsertWorkspaceProject(first.projects, "/Users/nolangrant/Other/", "/Users/nolangrant/Awaken");

    expect(first.projectId).toBe(second.projectId);
    expect(second.projects).toHaveLength(1);
    expect(second.projects[0]).toMatchObject({
      label: "Other",
      path: "/Users/nolangrant/Other",
    });
  });

  it("assigns new threads to the selected project with default fallback", () => {
    const mapping = assignThreadToProject({}, "thread-1", "workspace:%2Ftmp%2Fdemo");

    expect(projectIdForThread("thread-1", mapping)).toBe("workspace:%2Ftmp%2Fdemo");
    expect(projectIdForThread("thread-2", mapping)).toBe(DEFAULT_WORKSPACE_PROJECT_ID);
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
    expect(html).toContain('rows="2"');
    expect(html).toContain("New Agent");
    expect(html).toContain("Add folder");
    expect(html).toContain("composer-bar-center");
    expect(html).not.toContain("Plan, Build, / for skills, @ for context");
    expect(html).not.toContain("composer-bar-dock");
    expect(html).not.toContain("Planning next moves");
    expect(html).not.toContain("Ready for a run");
    expect(html).not.toContain("No narrative yet");
    expect(html).not.toContain("等待运行");
    expect(html).not.toContain("run-stage-header");
    expect(html).not.toContain("run-timeline-shell");
  });

  it("uses the dock composer after a run exists or narrative appears", () => {
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
    const eightTurns = Array.from({ length: 8 }, (_, index) => `turn-${index + 1}`);
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

    const twentyTurns = Array.from({ length: 20 }, (_, index) => `turn-${index + 1}`);
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
    const fortyTurns = Array.from({ length: 40 }, (_, index) => `turn-${index + 1}`);
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
      expect.arrayContaining(["turn-1", "turn-19", "turn-20", "turn-21", "turn-40"]),
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
      { kind: "prompt", id: "prompt:thread-1:1", text: "你好" },
      { kind: "thinking", id: "thinking:1", text: "first thought", status: "done" },
      { kind: "answer", id: "answer:1", markdown: "first answer", status: "done" },
      { kind: "prompt", id: "prompt:thread-1:2", text: "再次告诉我" },
      { kind: "answer", id: "answer:2", markdown: "second answer", status: "active" },
    ]);

    expect(grouped.leadItems).toEqual([
      { kind: "thinking", id: "intro", text: "warmup", status: "done" },
    ]);
    expect(grouped.turns).toHaveLength(2);
    expect(grouped.turns[0].prompt.text).toBe("你好");
    expect(grouped.turns[0].items).toHaveLength(2);
    expect(grouped.turns[1].prompt.text).toBe("再次告诉我");
    expect(grouped.turns[1].items).toEqual([
      { kind: "answer", id: "answer:2", markdown: "second answer", status: "active" },
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

  it("measures the full floating composer dock reserve", () => {
    expect(
      measuredComposerDockHeight({
        dockHeight: 132,
        contentHeight: 82,
        paddingTop: 32,
        paddingBottom: 18,
      }),
    ).toBe(132);
    expect(
      measuredComposerDockHeight({
        dockHeight: 0,
        contentHeight: 82,
        paddingTop: 32,
        paddingBottom: 18,
      }),
    ).toBe(132);
    expect(
      measuredComposerDockHeight({
        dockHeight: -1,
        contentHeight: -1,
        paddingTop: 0,
        paddingBottom: 0,
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
  });

  it("keeps latest output visible without scrolling past the prompt anchor for short output", () => {
    expect(
      scrollTopForLatestTurnOutputTail({
        currentScrollTop: 260,
        containerTop: 100,
        outputBottom: 360,
        containerHeight: 640,
        bottomReserve: 120,
        anchorScrollTop: 480,
      }),
    ).toBe(480);
  });

  it("scrolls the latest turn tail above the composer when output overflows", () => {
    expect(
      scrollTopForLatestTurnOutputTail({
        currentScrollTop: 260,
        containerTop: 100,
        outputBottom: 980,
        containerHeight: 640,
        bottomReserve: 120,
        anchorScrollTop: 480,
      }),
    ).toBe(620);
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

  it("pauses latest output follow as soon as the user scrolls toward history", () => {
    expect(shouldPauseLatestOutputFollowForWheel({ deltaY: -12 })).toBe(true);
    expect(shouldPauseLatestOutputFollowForWheel({ deltaY: 12 })).toBe(false);
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: 12 })).toBe(true);
    expect(shouldRequestLatestOutputFollowResumeForWheel({ deltaY: -12 })).toBe(false);
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
        hasResumeIntent: false,
      }),
    ).toBe(false);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: true,
        isNearTail: true,
        hasResumeIntent: true,
      }),
    ).toBe(true);
    expect(
      shouldResumeLatestOutputFollow({
        isPausedByUser: false,
        isNearTail: true,
        hasResumeIntent: false,
      }),
    ).toBe(true);
  });
});
