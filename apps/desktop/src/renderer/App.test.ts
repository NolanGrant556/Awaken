import React from "react";
import { renderToString } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import {
  diffLineStats,
  resolvedDecisionForApproval,
  shouldUseCenterComposer,
} from "./workbenchPresentation";
import type { AwakenBridge } from "../shared/runtime";

function stubElectronBridge(): void {
  vi.stubGlobal("window", {
    awaken: {
      platform: "macos",
      getRun: vi.fn(),
      listThreads: vi.fn(async () => []),
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
});
