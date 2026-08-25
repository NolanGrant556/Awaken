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
  plan?: RunPlanView;
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
    if (!controllerRef.current) throw new Error("harness not rendered");
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
    act(() => root?.unmount());
    root = undefined;
    host.remove();
    vi.restoreAllMocks();
  });

  it("shows the task entry whenever the thread has a durable plan", () => {
    renderHarness({ threadId: "t-1" });
    expect(controller().entryVisible).toBe(false);

    renderHarness({ threadId: "t-1", plan: planView() });
    expect(controller().entryVisible).toBe(true);
  });

  it("keeps the entry and open panel through pause, continuation and completion updates", () => {
    renderHarness({ threadId: "t-1", plan: planView() });
    act(() => controller().togglePanel());

    renderHarness({
      threadId: "t-1",
      plan: planView({ version: 2, completedCount: 1, currentStep: 2 }),
    });
    expect(controller().entryVisible).toBe(true);
    expect(controller().panelOpen).toBe(true);

    renderHarness({
      threadId: "t-1",
      plan: planView({
        planId: "plan-continuation",
        version: 1,
        completedCount: 2,
        currentStep: 2,
        allCompleted: true,
      }),
    });
    expect(controller().entryVisible).toBe(true);
    expect(controller().panelOpen).toBe(true);
  });

  it("shows a replayed completed plan after returning to or restarting a thread", () => {
    renderHarness({
      threadId: "t-1",
      plan: planView({ completedCount: 2, allCompleted: true }),
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
