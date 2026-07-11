// @vitest-environment jsdom

import {
  act,
  createRef,
  forwardRef,
  useImperativeHandle,
  useState,
} from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RunNarrativeItem } from "./run-narrative";
import { useRunStageScrollController } from "./useRunStageScrollController";

const RUN_STAGE_TOP = 100;
const RUN_STAGE_HEIGHT = 640;
const RUN_STAGE_SCROLL_HEIGHT = 1_800;
const INITIAL_SCROLL_TOP = 500;
const FOLLOW_UP_PROMPT_OFFSET = 900;
const FIRST_PROMPT_ID = "prompt:thread-1:1";
const FOLLOW_UP_PROMPT_ID = "prompt:thread-1:2";

const initialNarrativeItems: RunNarrativeItem[] = [
  {
    kind: "prompt",
    id: FIRST_PROMPT_ID,
    text: "first prompt",
  },
  {
    kind: "answer",
    id: "answer:run-1:final",
    markdown: "first final answer",
    status: "done",
    isFinal: true,
  },
];

const followUpNarrativeItems: RunNarrativeItem[] = [
  ...initialNarrativeItems,
  {
    kind: "prompt",
    id: FOLLOW_UP_PROMPT_ID,
    text: "follow-up prompt",
  },
];

type ControllerHarnessHandle = {
  submitFollowUp: () => void;
};

const ControllerHarness = forwardRef<ControllerHarnessHandle>(
  function ControllerHarness(_props, ref) {
    const [narrativeItems, setNarrativeItems] = useState<RunNarrativeItem[]>(
      () => [...initialNarrativeItems],
    );
    const controller = useRunStageScrollController({
      showCenterComposer: false,
      narrativeItems,
    });

    useImperativeHandle(
      ref,
      () => ({
        submitFollowUp: () => {
          controller.prepareForRunStart();
          setNarrativeItems([...followUpNarrativeItems]);
        },
      }),
      [controller.prepareForRunStart],
    );

    return (
      <div data-run-stage ref={controller.setRunStageScrollElement}>
        {controller.narrativeSections.turns.map((turn) => (
          <section key={turn.id} data-execution-turn-id={turn.id}>
            <div className="execution-turn-header">{turn.prompt.text}</div>
            <div className="execution-turn-body">
              {turn.items.map((item) => (
                <div key={item.id} data-narrative-item-id={item.id} />
              ))}
            </div>
          </section>
        ))}
      </div>
    );
  },
);

function domRect(top: number, height: number): DOMRect {
  return {
    x: 0,
    y: top,
    top,
    right: 760,
    bottom: top + height,
    left: 0,
    width: 760,
    height,
    toJSON: () => ({}),
  };
}

describe("useRunStageScrollController prompt anchor timing", () => {
  let host: HTMLDivElement;
  let root: Root | undefined;
  let animationFrameCallbacks: FrameRequestCallback[];
  let scrollToMock: ReturnType<typeof vi.fn>;
  let originalScrollToDescriptor: PropertyDescriptor | undefined;

  beforeEach(() => {
    (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT = true;

    host = document.createElement("div");
    document.body.append(host);
    animationFrameCallbacks = [];

    vi.stubGlobal(
      "ResizeObserver",
      class ResizeObserverMock {
        observe(): void {}
        unobserve(): void {}
        disconnect(): void {}
      },
    );
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn((callback: FrameRequestCallback) => {
        animationFrameCallbacks.push(callback);
        return animationFrameCallbacks.length;
      }),
    );

    originalScrollToDescriptor = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "scrollTo",
    );
    scrollToMock = vi.fn(function (
      this: HTMLElement,
      options?: ScrollToOptions | number,
      y?: number,
    ) {
      const nextTop =
        typeof options === "number" ? (y ?? 0) : (options?.top ?? 0);
      this.scrollTop = nextTop;
    });
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: scrollToMock,
    });

    const nativeGetBoundingClientRect =
      HTMLElement.prototype.getBoundingClientRect;
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      function getBoundingClientRect(this: HTMLElement): DOMRect {
        if (this.matches("[data-run-stage]")) {
          return domRect(RUN_STAGE_TOP, RUN_STAGE_HEIGHT);
        }

        const turn = this.closest<HTMLElement>("[data-execution-turn-id]");
        const runStage = this.closest<HTMLElement>("[data-run-stage]");
        if (turn && runStage) {
          const promptOffset =
            turn.dataset.executionTurnId === FOLLOW_UP_PROMPT_ID
              ? FOLLOW_UP_PROMPT_OFFSET
              : 0;
          return domRect(
            RUN_STAGE_TOP + promptOffset - runStage.scrollTop,
            this.matches(".execution-turn-header") ? 60 : 120,
          );
        }

        return nativeGetBoundingClientRect.call(this);
      },
    );
  });

  afterEach(() => {
    if (root) {
      act(() => {
        root?.unmount();
      });
      root = undefined;
    }
    host.remove();

    if (originalScrollToDescriptor) {
      Object.defineProperty(
        HTMLElement.prototype,
        "scrollTo",
        originalScrollToDescriptor,
      );
    } else {
      delete (HTMLElement.prototype as Partial<HTMLElement>).scrollTo;
    }

    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    delete (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT?: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT;
  });

  it("anchors a submitted follow-up before the first animation frame without waiting for body output", () => {
    const harnessRef = createRef<ControllerHarnessHandle>();
    root = createRoot(host);
    act(() => {
      root?.render(<ControllerHarness ref={harnessRef} />);
    });

    const runStage = host.querySelector<HTMLElement>("[data-run-stage]");
    expect(runStage).not.toBeNull();
    Object.defineProperties(runStage, {
      clientHeight: {
        configurable: true,
        value: RUN_STAGE_HEIGHT,
      },
      scrollHeight: {
        configurable: true,
        value: RUN_STAGE_SCROLL_HEIGHT,
      },
    });
    runStage!.scrollTop = INITIAL_SCROLL_TOP;
    scrollToMock.mockClear();
    animationFrameCallbacks.length = 0;

    let followUpPromptTopAtFirstFrame: number | undefined;
    window.requestAnimationFrame(() => {
      followUpPromptTopAtFirstFrame = host
        .querySelector<HTMLElement>(
          `[data-execution-turn-id="${FOLLOW_UP_PROMPT_ID}"] .execution-turn-header`,
        )
        ?.getBoundingClientRect().top;
    });

    act(() => {
      harnessRef.current?.submitFollowUp();
    });

    const followUpTurn = host.querySelector<HTMLElement>(
      `[data-execution-turn-id="${FOLLOW_UP_PROMPT_ID}"]`,
    );
    expect(followUpTurn).not.toBeNull();
    expect(
      followUpTurn?.querySelector(".execution-turn-body")?.childElementCount,
    ).toBe(0);
    expect(followUpPromptTopAtFirstFrame).toBeUndefined();
    expect(scrollToMock).toHaveBeenCalledWith({
      top: FOLLOW_UP_PROMPT_OFFSET,
      behavior: "auto",
    });
    expect(runStage?.scrollTop).toBe(FOLLOW_UP_PROMPT_OFFSET);
    expect(
      followUpTurn
        ?.querySelector<HTMLElement>(".execution-turn-header")
        ?.getBoundingClientRect().top,
    ).toBe(RUN_STAGE_TOP);
  });
});
