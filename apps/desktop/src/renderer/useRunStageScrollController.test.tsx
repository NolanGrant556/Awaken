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

const followUpAnswerNarrativeItems: RunNarrativeItem[] = [
  ...followUpNarrativeItems,
  {
    kind: "answer",
    id: "answer:run-2:final",
    markdown: "follow-up final answer",
    status: "done",
    isFinal: true,
  },
];

type ControllerHarnessHandle = {
  submitFollowUp: () => void;
  appendFollowUpAnswer: () => void;
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
        appendFollowUpAnswer: () => {
          setNarrativeItems([...followUpAnswerNarrativeItems]);
        },
      }),
      [controller.prepareForRunStart],
    );

    return (
      <div
        data-run-stage
        data-return-to-latest-visible={String(controller.returnToLatestVisible)}
        ref={controller.setRunStageScrollElement}
        onScroll={controller.handleRunStageScroll}
      >
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

type ResizeObserverRecord = {
  callback: ResizeObserverCallback;
  targets: Element[];
};

describe("useRunStageScrollController prompt anchor timing", () => {
  let host: HTMLDivElement;
  let root: Root | undefined;
  let animationFrameCallbacks: FrameRequestCallback[];
  let resizeObserverRecords: ResizeObserverRecord[];
  let scrollToMock: ReturnType<typeof vi.fn>;
  let originalScrollToDescriptor: PropertyDescriptor | undefined;
  let followUpPromptOffset: number;

  beforeEach(() => {
    (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT = true;

    host = document.createElement("div");
    document.body.append(host);
    animationFrameCallbacks = [];
    resizeObserverRecords = [];
    followUpPromptOffset = FOLLOW_UP_PROMPT_OFFSET;

    vi.stubGlobal(
      "ResizeObserver",
      class ResizeObserverMock {
        private readonly record: ResizeObserverRecord;

        constructor(callback: ResizeObserverCallback) {
          this.record = { callback, targets: [] };
          resizeObserverRecords.push(this.record);
        }

        observe(target: Element): void {
          this.record.targets.push(target);
        }

        unobserve(target: Element): void {
          this.record.targets = this.record.targets.filter(
            (observed) => observed !== target,
          );
        }

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
              ? followUpPromptOffset
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

  it("keeps the follow-up prompt anchored when an earlier media turn changes height", () => {
    const harnessRef = createRef<ControllerHarnessHandle>();
    root = createRoot(host);
    act(() => {
      root?.render(<ControllerHarness ref={harnessRef} />);
    });

    const runStage = host.querySelector<HTMLElement>("[data-run-stage]");
    expect(runStage).not.toBeNull();
    Object.defineProperties(runStage, {
      clientHeight: { configurable: true, value: RUN_STAGE_HEIGHT },
      scrollHeight: { configurable: true, value: RUN_STAGE_SCROLL_HEIGHT },
    });
    runStage!.scrollTop = INITIAL_SCROLL_TOP;
    act(() => {
      harnessRef.current?.submitFollowUp();
    });

    scrollToMock.mockClear();
    followUpPromptOffset += 180;
    Object.defineProperty(runStage, "scrollHeight", {
      configurable: true,
      value: RUN_STAGE_SCROLL_HEIGHT + 180,
    });
    expect(runStage?.scrollTop).toBe(FOLLOW_UP_PROMPT_OFFSET);
    expect(
      runStage
        ?.querySelector<HTMLElement>(
          `[data-execution-turn-id="${FOLLOW_UP_PROMPT_ID}"] .execution-turn-header`,
        )
        ?.getBoundingClientRect().top,
    ).toBe(RUN_STAGE_TOP + 180);
    const earlierTurn = runStage?.querySelector(
      `[data-execution-turn-id="${FIRST_PROMPT_ID}"]`,
    );
    const record = [...resizeObserverRecords].reverse().find((entry) =>
      entry.targets.includes(earlierTurn as Element),
    );
    expect(record).toBeDefined();
    act(() => {
      record?.callback([], {} as ResizeObserver);
    });

    expect(scrollToMock).toHaveBeenCalledWith({
      top: FOLLOW_UP_PROMPT_OFFSET + 180,
      behavior: "auto",
    });
    expect(
      runStage
        ?.querySelector<HTMLElement>(
          `[data-execution-turn-id="${FOLLOW_UP_PROMPT_ID}"] .execution-turn-header`,
        )
        ?.getBoundingClientRect().top,
    ).toBe(RUN_STAGE_TOP);
  });

  describe("streaming layout follow", () => {
    function defineRunStageScrollGeometry(
      element: HTMLElement,
      scrollHeight: number,
    ): void {
      Object.defineProperties(element, {
        clientHeight: { configurable: true, value: RUN_STAGE_HEIGHT },
        scrollHeight: { configurable: true, value: scrollHeight },
      });
    }

    function flushAnimationFrames(): void {
      const callbacks = animationFrameCallbacks.splice(0);
      for (const callback of callbacks) {
        callback(16);
      }
    }

    function triggerTurnResizeObserver(
      runStage: HTMLElement,
      turnId: string,
    ): void {
      const observedTurn = runStage.querySelector(
        `[data-execution-turn-id="${turnId}"]`,
      );
      const record = [...resizeObserverRecords].reverse().find((entry) =>
        entry.targets.includes(observedTurn as Element),
      );
      expect(record).toBeDefined();
      record?.callback([], {} as ResizeObserver);
    }

    function renderFollowingController(): { runStage: HTMLElement } {
      const harnessRef = createRef<ControllerHarnessHandle>();
      root = createRoot(host);
      act(() => {
        root?.render(<ControllerHarness ref={harnessRef} />);
      });

      const runStage = host.querySelector<HTMLElement>("[data-run-stage]");
      expect(runStage).not.toBeNull();
      defineRunStageScrollGeometry(runStage!, RUN_STAGE_SCROLL_HEIGHT);
      runStage!.scrollTop = INITIAL_SCROLL_TOP;

      act(() => {
        harnessRef.current?.submitFollowUp();
      });
      act(() => {
        harnessRef.current?.appendFollowUpAnswer();
      });
      act(() => {
        flushAnimationFrames();
      });
      return { runStage: runStage! };
    }

    it("re-pins to the latest tail when the latest turn resize observer fires while following", () => {
      const { runStage } = renderFollowingController();
      expect(runStage.scrollTop).toBe(RUN_STAGE_SCROLL_HEIGHT - RUN_STAGE_HEIGHT);
      expect(runStage.dataset.returnToLatestVisible).toBe("false");

      scrollToMock.mockClear();
      // mermaid 渲染、SVG/图片注入等异步增高，触发布局 observer。
      defineRunStageScrollGeometry(runStage, 2_400);
      act(() => {
        triggerTurnResizeObserver(runStage, FOLLOW_UP_PROMPT_ID);
      });

      expect(scrollToMock).toHaveBeenCalledWith({
        top: 2_400 - RUN_STAGE_HEIGHT,
        behavior: "auto",
      });
      expect(runStage.scrollTop).toBe(2_400 - RUN_STAGE_HEIGHT);
    });

    it("does not re-pin from the resize observer after the user pauses follow", () => {
      const { runStage } = renderFollowingController();

      act(() => {
        runStage.dispatchEvent(
          new WheelEvent("wheel", { deltaY: -12, bubbles: true }),
        );
      });
      expect(runStage.dataset.returnToLatestVisible).toBe("true");

      scrollToMock.mockClear();
      defineRunStageScrollGeometry(runStage, 3_000);
      act(() => {
        triggerTurnResizeObserver(runStage, FOLLOW_UP_PROMPT_ID);
      });

      expect(scrollToMock).not.toHaveBeenCalled();
      expect(runStage.dataset.returnToLatestVisible).toBe("true");
    });

    it("keeps the latest tail stable when an earlier media turn resizes while following", () => {
      const { runStage } = renderFollowingController();
      const promptTopBefore = runStage
        .querySelector<HTMLElement>(
          `[data-execution-turn-id="${FOLLOW_UP_PROMPT_ID}"] .execution-turn-header`,
        )
        ?.getBoundingClientRect().top;

      scrollToMock.mockClear();
      followUpPromptOffset += 220;
      defineRunStageScrollGeometry(runStage, RUN_STAGE_SCROLL_HEIGHT + 220);
      act(() => {
        triggerTurnResizeObserver(runStage, FIRST_PROMPT_ID);
      });

      expect(scrollToMock).toHaveBeenCalledWith({
        top: RUN_STAGE_SCROLL_HEIGHT + 220 - RUN_STAGE_HEIGHT,
        behavior: "auto",
      });
      expect(
        runStage
          .querySelector<HTMLElement>(
            `[data-execution-turn-id="${FOLLOW_UP_PROMPT_ID}"] .execution-turn-header`,
          )
          ?.getBoundingClientRect().top,
      ).toBe(promptTopBefore);
    });

    it("ignores passive scroll clamping at the bottom but still pauses for a real upward scroll", () => {
      const { runStage } = renderFollowingController();
      expect(runStage.dataset.returnToLatestVisible).toBe("false");

      // scrollHeight 收缩时浏览器把贴底的 scrollTop 钳到新的 max，不算用户上滑。
      defineRunStageScrollGeometry(runStage, 1_200);
      runStage.scrollTop = 1_200 - RUN_STAGE_HEIGHT;
      act(() => {
        runStage.dispatchEvent(new Event("scroll"));
      });
      expect(runStage.dataset.returnToLatestVisible).toBe("false");

      // 离开底部之后的向上滚动仍然是用户暂停。
      runStage.scrollTop = 400;
      act(() => {
        runStage.dispatchEvent(new Event("scroll"));
      });
      expect(runStage.dataset.returnToLatestVisible).toBe("true");
    });
  });
});
