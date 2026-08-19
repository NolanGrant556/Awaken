// @vitest-environment jsdom

import {
  act,
  forwardRef,
  useImperativeHandle,
} from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RunPlanView } from "./runtime/reducer";
import {
  RUN_PLAN_TERMINAL_HIDE_DELAY_MS,
  useRunPlanEntry,
  type RunPlanEntryController,
} from "./useRunPlanEntry";

function planView(overrides: Partial<RunPlanView> = {}): RunPlanView {
  return {
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
    ...overrides,
  };
}

type HarnessProps = {
  threadId?: string;
  runId?: string;
  plan?: RunPlanView;
  isBusy: boolean;
};

const EntryHarness = forwardRef<RunPlanEntryController, HarnessProps>(
  function EntryHarness(props, ref) {
    const controller = useRunPlanEntry(props);
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
    if (!controllerRef.current) {
      throw new Error("harness not rendered");
    }
    return controllerRef.current;
  }

  beforeEach(() => {
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
    act(() => {
      root?.unmount();
    });
    root = undefined;
    host.remove();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("shows the entry only once a live run has a plan snapshot", () => {
    renderHarness({ threadId: "t-1", runId: "run-1", isBusy: true });
    expect(controller().entryVisible).toBe(false);

    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView(),
    });
    expect(controller().entryVisible).toBe(true);
  });

  it("keeps the entry visible through in-place updates and full completion while live", () => {
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView(),
    });
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView({
        version: 2,
        items: [
          { step: "第一步", activeForm: "正在做第一步", status: "completed" },
          { step: "第二步", activeForm: "正在做第二步", status: "in_progress" },
        ],
        completedCount: 1,
        currentStep: 2,
      }),
    });
    expect(controller().entryVisible).toBe(true);

    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView({
        version: 3,
        items: [
          { step: "第一步", activeForm: "正在做第一步", status: "completed" },
          { step: "第二步", activeForm: "正在做第二步", status: "completed" },
        ],
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
      }),
    });
    expect(controller().entryVisible).toBe(true);
  });

  it("keeps the entry at live terminal and hides it 5s later, closing the panel", () => {
    vi.useFakeTimers();
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView(),
    });
    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);

    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: false,
      plan: planView(),
    });
    expect(controller().entryVisible).toBe(true);
    expect(controller().panelOpen).toBe(true);

    act(() => {
      vi.advanceTimersByTime(RUN_PLAN_TERMINAL_HIDE_DELAY_MS - 1);
    });
    expect(controller().entryVisible).toBe(true);

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(controller().entryVisible).toBe(false);
    expect(controller().panelOpen).toBe(false);
  });

  it("never shows the live entry for a terminal plan that was not observed live", () => {
    vi.useFakeTimers();
    // Replay/restart restore: the snapshot appears already terminal.
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: false,
      plan: planView({ version: 3, allCompleted: true }),
    });
    expect(controller().entryVisible).toBe(false);

    act(() => {
      vi.advanceTimersByTime(RUN_PLAN_TERMINAL_HIDE_DELAY_MS * 2);
    });
    expect(controller().entryVisible).toBe(false);
  });

  it("resets live observation and open state when the run changes", () => {
    vi.useFakeTimers();
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView(),
    });
    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);

    // A new run starts without a plan: the previous entry must not linger.
    renderHarness({ threadId: "t-1", runId: "run-2", isBusy: true });
    expect(controller().entryVisible).toBe(false);
    expect(controller().panelOpen).toBe(false);

    // The previous run's plan arriving already terminal (e.g. transcript
    // replay advancing across runs) must not resurrect the live entry.
    renderHarness({
      threadId: "t-1",
      runId: "run-2",
      isBusy: false,
      plan: planView(),
    });
    expect(controller().entryVisible).toBe(false);
  });

  it("does not leak the entry across thread switches", () => {
    renderHarness({
      threadId: "thread-a",
      runId: "run-a",
      isBusy: true,
      plan: planView(),
    });
    act(() => controller().togglePanel());
    expect(controller().entryVisible).toBe(true);
    expect(controller().panelOpen).toBe(true);

    renderHarness({ threadId: "thread-b", runId: "run-b", isBusy: false });
    expect(controller().entryVisible).toBe(false);
    expect(controller().panelOpen).toBe(false);

    // Switching back to a thread whose run finished while away: terminal
    // snapshot was not observed live, so the live entry stays hidden.
    renderHarness({
      threadId: "thread-a",
      runId: "run-a",
      isBusy: false,
      plan: planView(),
    });
    expect(controller().entryVisible).toBe(false);
  });

  it("toggles the panel and restores focus to the trigger on close", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0);
      return 0;
    });
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView(),
    });

    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);

    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(false);
    const trigger = host.querySelector("button");
    expect(document.activeElement).toBe(trigger);
  });

  it("closes on Escape and on outside pointer down with focus restore", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0);
      return 0;
    });
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView(),
    });

    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", cancelable: true }),
      );
    });
    expect(controller().panelOpen).toBe(false);

    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);
    act(() => {
      document.body.dispatchEvent(
        new MouseEvent("mousedown", { bubbles: true }),
      );
    });
    expect(controller().panelOpen).toBe(false);
  });

  it("keeps the panel open for pointer down inside the panel or trigger", () => {
    renderHarness({
      threadId: "t-1",
      runId: "run-1",
      isBusy: true,
      plan: planView(),
    });
    act(() => controller().togglePanel());
    expect(controller().panelOpen).toBe(true);

    const panel = host.querySelector("[data-testid='panel']");
    const trigger = host.querySelector("button");
    act(() => {
      panel?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    });
    expect(controller().panelOpen).toBe(true);
    act(() => {
      trigger?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    });
    expect(controller().panelOpen).toBe(true);
  });
});
