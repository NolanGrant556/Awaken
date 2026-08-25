// @vitest-environment jsdom

import { act, forwardRef, useImperativeHandle } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RunPlanView } from "./runtime/reducer";
import {
  useRunPlanEntry,
  type RunPlanEntryController,
} from "./useRunPlanEntry";

function planView(overrides: Partial<RunPlanView> = {}): RunPlanView {
  return {
    runId: "run-1",
    planId: "plan-1",
    version: 1,
    items: [
      { step: "第一步", activeForm: "正在做第一步", status: "in_progress" },
      { step: "第二步", activeForm: "正在做第二步", status: "pending" },
    ],
    completedCount: 0,
    totalCount: 2,
    currentStep: 1,
    allCompleted: false,
    updatedAtMs: 9_000,
    ...overrides,
  };
}

type HarnessProps = {
  threadId?: string;
  plan?: RunPlanView;
  currentRunId?: string;
  phase?: "idle" | "starting" | "running" | "reconnecting" | "cancelling" | "done" | "error" | "cancelled";
  runStartedAtMs?: number;
  runCompletedAtMs?: number;
};

const EntryHarness = forwardRef<RunPlanEntryController, HarnessProps>(
  function EntryHarness(props, ref) {
    const controller = useRunPlanEntry({ phase: "running", ...props });
    useImperativeHandle(ref, () => controller);
    return (
      <div>
        <button ref={controller.triggerRef} type="button">
          trigger
        </button>
        {controller.panelOpen ? (
          <div ref={controller.panelRef} data-testid="panel" />
        ) : null}
      </div>
    );
  },
);

describe("useRunPlanEntry", () => {
  let host: HTMLDivElement;
  let root: Root | undefined;
  let controllerRef: React.RefObject<RunPlanEntryController | null>;

  function renderHarness(props: HarnessProps): void {
    act(() => {
      root?.render(<EntryHarness ref={controllerRef} {...props} />);
    });
  }

  function controller(): RunPlanEntryController {
    if (!controllerRef.current) throw new Error("harness not rendered");
    return controllerRef.current;
  }

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(10_000);
    (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    host = document.createElement("div");
    document.body.append(host);
    controllerRef = { current: null };
    act(() => {
      root = createRoot(host);
    });
  });

  afterEach(() => {
    act(() => root?.unmount());
    root = undefined;
    host.remove();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("shows the task entry whenever the thread has a durable plan", () => {
    renderHarness({ threadId: "t-1" });
    expect(controller().entryVisible).toBe(false);

    renderHarness({ threadId: "t-1", plan: planView() });
    expect(controller().entryVisible).toBe(true);
  });

  it("keeps the entry and open panel through active, paused and pre-terminal completion updates", () => {
    renderHarness({ threadId: "t-1", currentRunId: "run-1", plan: planView() });
    act(() => controller().togglePanel());

    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "cancelled",
      plan: planView({ version: 2, completedCount: 1, currentStep: 2 }),
    });
    expect(controller().entryVisible).toBe(true);
    expect(controller().panelOpen).toBe(true);

    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "running",
      plan: planView({
        version: 3,
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
      }),
    });
    expect(controller().entryVisible).toBe(true);
    expect(controller().panelOpen).toBe(true);
  });

  it("keeps a completed plan for five seconds after successful terminal, then hides it", () => {
    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "done",
      runCompletedAtMs: 10_000,
      plan: planView({
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
      }),
    });
    expect(controller().entryVisible).toBe(true);
    act(() => controller().togglePanel());

    act(() => vi.advanceTimersByTime(4_999));
    expect(controller().entryVisible).toBe(true);
    expect(controller().panelOpen).toBe(true);

    act(() => vi.advanceTimersByTime(1));
    expect(controller().entryVisible).toBe(false);
    expect(controller().panelOpen).toBe(false);
  });

  it("hides an already-expired completed replay before it can remain in the composer", () => {
    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "done",
      runCompletedAtMs: 2_000,
      plan: planView({
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
        updatedAtMs: 1_000,
      }),
    });
    expect(controller().entryVisible).toBe(false);
  });

  it("cancels completion hiding and reappears when a newer active snapshot arrives", () => {
    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "done",
      runCompletedAtMs: 10_000,
      plan: planView({
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
      }),
    });
    act(() => vi.advanceTimersByTime(2_000));

    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "running",
      plan: planView({ version: 2, updatedAtMs: 12_000 }),
    });
    expect(controller().entryVisible).toBe(true);

    act(() => vi.advanceTimersByTime(5_000));
    expect(controller().entryVisible).toBe(true);
  });

  it("does not revive a completed previous plan while a new run is starting", () => {
    renderHarness({
      threadId: "t-1",
      phase: "starting",
      runStartedAtMs: 10_000,
      plan: planView({
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
        updatedAtMs: 9_000,
      }),
    });
    expect(controller().entryVisible).toBe(false);

    renderHarness({
      threadId: "t-1",
      phase: "starting",
      runStartedAtMs: 10_000,
      plan: planView({ updatedAtMs: 9_000 }),
    });
    expect(controller().entryVisible).toBe(true);
  });

  it("allows completed entry dismissal and shows the next version again", () => {
    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "running",
      plan: planView({ completedCount: 2, currentStep: 2, allCompleted: true }),
    });
    act(() => controller().dismissCompletedEntry());
    expect(controller().entryVisible).toBe(false);

    renderHarness({
      threadId: "t-1",
      currentRunId: "run-1",
      phase: "running",
      plan: planView({
        version: 2,
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
        updatedAtMs: 10_000,
      }),
    });
    expect(controller().entryVisible).toBe(true);
  });

  it("closes the overlay on thread switch without leaking the previous plan", () => {
    renderHarness({ threadId: "thread-a", plan: planView() });
    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);

    renderHarness({ threadId: "thread-b" });
    expect(controller().entryVisible).toBe(false);
    expect(controller().panelOpen).toBe(false);
  });

  it("toggles, closes on Escape/outside pointer and restores trigger focus", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0);
      return 0;
    });
    renderHarness({ threadId: "t-1", plan: planView() });

    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", cancelable: true }),
      );
    });
    expect(controller().panelOpen).toBe(false);
    expect(document.activeElement).toBe(host.querySelector("button"));

    act(() => controller().togglePanel());
    act(() => {
      document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    });
    expect(controller().panelOpen).toBe(false);
  });
});
