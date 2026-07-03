import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { UIEvent as ReactUIEvent } from "react";
import type { RunNarrativeItem } from "./run-narrative";
import {
  activeTurnIdForAnchor,
  anchoredTurnMinHeight,
  composerBottomSafeArea,
  composerContentBottomInset,
  groupNarrativeByTurns,
  isNearLatestTurnOutputTail,
  isNearScrollBottom,
  latestTurnMinHeightBottomReserve,
  latestTurnPromptAnchorSlack,
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
} from "./workbenchPresentation";

const DEFAULT_PROMPT_ANCHOR_OFFSET = 0;
const RUN_STAGE_COMPOSER_SAFE_GAP = 72;
const LATEST_OUTPUT_FOLLOW_THRESHOLD = 48;

type RunStageScrollControllerOptions = {
  showCenterComposer: boolean;
  narrativeItems: RunNarrativeItem[];
};

function promptAnchorOffset(element: HTMLElement): number {
  const value = window
    .getComputedStyle(element)
    .getPropertyValue("--aw-prompt-anchor-offset")
    .trim();
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : DEFAULT_PROMPT_ANCHOR_OFFSET;
}

function turnElements(element: HTMLElement): HTMLElement[] {
  return Array.from(
    element.querySelectorAll<HTMLElement>("[data-execution-turn-id]"),
  );
}

function findTurnElement(element: HTMLElement, turnId: string): HTMLElement | undefined {
  return turnElements(element).find((turn) => turn.dataset.executionTurnId === turnId);
}

function turnPromptAnchorElement(turn: HTMLElement): HTMLElement {
  return turn.querySelector<HTMLElement>(".execution-turn-header") ?? turn;
}

function latestTurnOutputBottom(turn: HTMLElement): number {
  const body = turn.querySelector<HTMLElement>(".execution-turn-body");
  const latestBodyItem = body?.lastElementChild;
  if (latestBodyItem instanceof HTMLElement) {
    const footer = latestBodyItem.querySelector<HTMLElement>(".assistant-message-footer");
    if (footer) {
      return footer.getBoundingClientRect().bottom;
    }
    return latestBodyItem.getBoundingClientRect().bottom;
  }

  const header = turn.querySelector<HTMLElement>(".execution-turn-header");
  return (header ?? turn).getBoundingClientRect().bottom;
}

function turnHasBodyItems(turn: HTMLElement): boolean {
  const body = turn.querySelector<HTMLElement>(".execution-turn-body");
  return Boolean(body?.firstElementChild);
}

export function useRunStageScrollController({
  showCenterComposer,
  narrativeItems,
}: RunStageScrollControllerOptions) {
  const pendingPromptAnchorRef = useRef(false);
  const followLatestOutputRef = useRef(false);
  const latestOutputFollowPausedByUserRef = useRef(false);
  const latestOutputFollowResumeIntentRef = useRef(false);
  const programmaticRunStageScrollTopRef = useRef<number | null>(null);
  const runStageLastScrollTopRef = useRef<number | null>(null);
  const runStageTouchStartYRef = useRef<number | null>(null);
  const runStageInputIntentCleanupRef = useRef<(() => void) | null>(null);
  const runStageResizeObserverRef = useRef<ResizeObserver | null>(null);
  const runStageScrollRef = useRef<HTMLDivElement | null>(null);
  const composerDockRef = useRef<HTMLDivElement | null>(null);

  const [activeTurnId, setActiveTurnId] = useState<string | undefined>();
  const [latestTurnMinHeightPx, setLatestTurnMinHeightPx] = useState(0);
  const [composerContentBottomInsetPx, setComposerContentBottomInsetPx] = useState(0);
  const [runStageViewportHeightPx, setRunStageViewportHeightPx] = useState(0);

  const narrativeSections = useMemo(
    () => groupNarrativeByTurns(narrativeItems),
    [narrativeItems],
  );
  const latestTurn = narrativeSections.turns[narrativeSections.turns.length - 1];
  const latestPromptId = latestTurn ? latestTurn.id : undefined;
  const hasPromptNarrative = narrativeSections.turns.length > 0;
  const composerContentInsetPx = hasPromptNarrative ? composerContentBottomInsetPx : 0;
  const composerOutputGapPx = hasPromptNarrative ? RUN_STAGE_COMPOSER_SAFE_GAP : 0;
  const composerSafeAreaPx = composerBottomSafeArea({
    composerHeight: composerContentInsetPx,
    safeGap: composerOutputGapPx,
  });
  const activeWorkbenchTurnId =
    activeTurnId &&
    narrativeSections.turns.some((turn) => turn.id === activeTurnId)
      ? activeTurnId
      : latestPromptId;

  const applyRunStageComposerVars = useCallback((element: HTMLElement): void => {
    const targets = [element, element.parentElement].filter(
      (target): target is HTMLElement => target instanceof HTMLElement,
    );
    for (const target of targets) {
      target.style.setProperty("--aw-composer-safe-area", `${composerSafeAreaPx}px`);
      target.style.setProperty("--aw-composer-content-inset", `${composerContentInsetPx}px`);
      target.style.setProperty("--aw-output-gap", `${composerOutputGapPx}px`);
    }
  }, [composerContentInsetPx, composerOutputGapPx, composerSafeAreaPx]);

  const resetScrollFollowState = useCallback((nextPendingPromptAnchor: boolean): void => {
    pendingPromptAnchorRef.current = nextPendingPromptAnchor;
    followLatestOutputRef.current = nextPendingPromptAnchor;
    latestOutputFollowPausedByUserRef.current = false;
    latestOutputFollowResumeIntentRef.current = false;
    setLatestTurnMinHeightPx(0);
  }, []);

  const prepareForRunStart = useCallback((): void => {
    resetScrollFollowState(true);
  }, [resetScrollFollowState]);

  const prepareForThreadSwitch = useCallback((): void => {
    resetScrollFollowState(false);
  }, [resetScrollFollowState]);

  const prepareForNewTask = useCallback((): void => {
    resetScrollFollowState(false);
  }, [resetScrollFollowState]);

  function activeTurnMarkerFromScroll(element: HTMLElement): string | undefined {
    const containerTop = element.getBoundingClientRect().top;
    const anchorY = containerTop + promptAnchorOffset(element) + 1;
    return activeTurnIdForAnchor({
      anchorY,
      turns: turnElements(element)
        .map((turn) => ({
          id: turn.dataset.executionTurnId ?? "",
          top: turn.getBoundingClientRect().top,
        }))
        .filter((turn) => turn.id.length > 0),
    });
  }

  function syncActiveTurnFromScroll(element: HTMLElement): void {
    const nextActiveId = activeTurnMarkerFromScroll(element);
    if (!nextActiveId) {
      return;
    }
    setActiveTurnId((current) => current === nextActiveId ? current : nextActiveId);
  }

  function pauseLatestOutputFollowForUser(): void {
    if (shouldCancelPendingPromptAnchorForScrollIntent({ hasDirectUserInput: true })) {
      pendingPromptAnchorRef.current = false;
    }
    latestOutputFollowPausedByUserRef.current = true;
    latestOutputFollowResumeIntentRef.current = false;
    followLatestOutputRef.current = false;
  }

  function pauseLatestOutputFollowForScroll(): void {
    if (shouldCancelPendingPromptAnchorForScrollIntent({ hasDirectUserInput: false })) {
      pendingPromptAnchorRef.current = false;
    }
    latestOutputFollowPausedByUserRef.current = true;
    latestOutputFollowResumeIntentRef.current = false;
    followLatestOutputRef.current = false;
  }

  function measureLatestTurnMinHeight(promptId: string): void {
    const element = runStageScrollRef.current;
    const turn = element ? findTurnElement(element, promptId) : null;
    if (!element || !turn) {
      setLatestTurnMinHeightPx(0);
      return;
    }

    const bottomReserve = latestTurnMinHeightBottomReserve({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending: pendingPromptAnchorRef.current,
      outputGap: composerOutputGapPx,
    });
    const promptAnchorSlack = latestTurnPromptAnchorSlack({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending: pendingPromptAnchorRef.current,
      outputGap: composerOutputGapPx,
    });

    setLatestTurnMinHeightPx(
      Math.ceil(
        anchoredTurnMinHeight({
          containerHeight: element.clientHeight,
          anchorOffset: promptAnchorOffset(element),
          bottomReserve,
        }) + promptAnchorSlack,
      ),
    );
  }

  function scrollRunStageTo(element: HTMLElement, scrollTop: number): void {
    const maxScrollTop = Math.max(0, element.scrollHeight - element.clientHeight);
    const nextScrollTop = Math.min(maxScrollTop, Math.max(0, scrollTop));
    programmaticRunStageScrollTopRef.current = nextScrollTop;
    runStageLastScrollTopRef.current = nextScrollTop;
    element.scrollTo({
      top: nextScrollTop,
      behavior: "auto",
    });
    window.requestAnimationFrame(() => {
      if (programmaticRunStageScrollTopRef.current === nextScrollTop) {
        programmaticRunStageScrollTopRef.current = null;
      }
    });
  }

  function scrollPromptToAnchor(promptId: string): void {
    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }

    const turn = findTurnElement(element, promptId);
    if (!turn) {
      return;
    }

    const elementRect = element.getBoundingClientRect();
    const turnRect = turnPromptAnchorElement(turn).getBoundingClientRect();
    scrollRunStageTo(
      element,
      scrollTopForPromptAnchor({
        currentScrollTop: element.scrollTop,
        containerTop: elementRect.top,
        promptTop: turnRect.top,
        anchorOffset: promptAnchorOffset(element),
      }),
    );
  }

  function scrollLatestTurnToScrollTail(promptId: string): void {
    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }

    const turn = findTurnElement(element, promptId);
    if (!turn) {
      return;
    }

    if (
      !shouldFollowLatestTurnScrollTail({
        hasBodyItems: turnHasBodyItems(turn),
        isPromptAnchorPending: pendingPromptAnchorRef.current,
      })
    ) {
      return;
    }

    const nextScrollTop = scrollTopForLatestTurnScrollTail({
      maxScrollTop: Math.max(0, element.scrollHeight - element.clientHeight),
    });

    if (Math.abs(nextScrollTop - element.scrollTop) < 1) {
      return;
    }

    scrollRunStageTo(element, nextScrollTop);
  }

  const setRunStageScrollElement = useCallback((element: HTMLDivElement | null) => {
    runStageInputIntentCleanupRef.current?.();
    runStageInputIntentCleanupRef.current = null;
    runStageResizeObserverRef.current?.disconnect();
    runStageResizeObserverRef.current = null;
    runStageScrollRef.current = element;
    if (!element) {
      programmaticRunStageScrollTopRef.current = null;
      runStageLastScrollTopRef.current = null;
      setRunStageViewportHeightPx((current) => current === 0 ? current : 0);
      return;
    }
    runStageLastScrollTopRef.current = element.scrollTop;
    applyRunStageComposerVars(element);

    const syncRunStageViewportHeight = () => {
      const nextHeight = element.clientHeight;
      setRunStageViewportHeightPx((current) => current === nextHeight ? current : nextHeight);
    };
    syncRunStageViewportHeight();
    if (typeof ResizeObserver !== "undefined") {
      const resizeObserver = new ResizeObserver(syncRunStageViewportHeight);
      resizeObserver.observe(element);
      runStageResizeObserverRef.current = resizeObserver;
    }

    const handleWheel = (event: WheelEvent) => {
      if (shouldPauseLatestOutputFollowForWheel({ deltaY: event.deltaY })) {
        pauseLatestOutputFollowForUser();
        return;
      }
      if (shouldRequestLatestOutputFollowResumeForWheel({ deltaY: event.deltaY })) {
        latestOutputFollowResumeIntentRef.current = true;
      }
    };
    const handlePointerDown = () => {
      pauseLatestOutputFollowForUser();
    };
    const handleTouchStart = (event: TouchEvent) => {
      runStageTouchStartYRef.current = event.touches[0]?.clientY ?? null;
    };
    const handleTouchMove = (event: TouchEvent) => {
      const startY = runStageTouchStartYRef.current;
      const currentY = event.touches[0]?.clientY;
      if (
        startY !== null &&
        currentY !== undefined &&
        shouldPauseLatestOutputFollowForTouchMove({ startY, currentY })
      ) {
        pauseLatestOutputFollowForUser();
        return;
      }
      if (
        startY !== null &&
        currentY !== undefined &&
        shouldRequestLatestOutputFollowResumeForTouchMove({ startY, currentY })
      ) {
        latestOutputFollowResumeIntentRef.current = true;
      }
    };

    element.addEventListener("wheel", handleWheel, { passive: true });
    element.addEventListener("pointerdown", handlePointerDown, { passive: true });
    element.addEventListener("touchstart", handleTouchStart, { passive: true });
    element.addEventListener("touchmove", handleTouchMove, { passive: true });
    runStageInputIntentCleanupRef.current = () => {
      element.removeEventListener("wheel", handleWheel);
      element.removeEventListener("pointerdown", handlePointerDown);
      element.removeEventListener("touchstart", handleTouchStart);
      element.removeEventListener("touchmove", handleTouchMove);
    };
  }, [applyRunStageComposerVars]);

  function handleTurnMarkerClick(turnId: string): void {
    pauseLatestOutputFollowForUser();
    setActiveTurnId(turnId);
    scrollPromptToAnchor(turnId);
  }

  function handleRunStageScroll(event: ReactUIEvent<HTMLDivElement>): void {
    const element = event.currentTarget;
    const previousScrollTop = runStageLastScrollTopRef.current;
    const nextScrollTop = element.scrollTop;
    runStageLastScrollTopRef.current = nextScrollTop;
    const programmaticScrollTop = programmaticRunStageScrollTopRef.current;
    const isProgrammaticScroll =
      programmaticScrollTop !== null &&
      Math.abs(nextScrollTop - programmaticScrollTop) < 2;
    if (isProgrammaticScroll) {
      programmaticRunStageScrollTopRef.current = null;
    }

    syncActiveTurnFromScroll(element);

    if (isProgrammaticScroll) {
      return;
    }

    if (!latestPromptId) {
      followLatestOutputRef.current = false;
      return;
    }

    const turn = findTurnElement(element, latestPromptId);
    if (!turn) {
      followLatestOutputRef.current = false;
      return;
    }

    if (
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop,
        nextScrollTop,
      })
    ) {
      pauseLatestOutputFollowForScroll();
      return;
    }

    const elementRect = element.getBoundingClientRect();
    const isNearTail = isNearLatestTurnOutputTail({
      containerTop: elementRect.top,
      outputBottom: latestTurnOutputBottom(turn),
      containerHeight: element.clientHeight,
      bottomReserve: composerOutputGapPx,
      threshold: LATEST_OUTPUT_FOLLOW_THRESHOLD,
    });
    const isAtScrollBottom = isNearScrollBottom({
      scrollTop: nextScrollTop,
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
      threshold: 2,
    });
    const wasFollowingLatestOutput = followLatestOutputRef.current;
    const hadResumeIntent = latestOutputFollowResumeIntentRef.current;
    const isPausedByUser = latestOutputFollowPausedByUserRef.current;
    const canFollowLatestOutput =
      wasFollowingLatestOutput || hadResumeIntent || (!isPausedByUser && isAtScrollBottom);
    const shouldResume = shouldResumeLatestOutputFollow({
      isPausedByUser,
      isNearTail,
      isAtScrollBottom,
      hasResumeIntent: hadResumeIntent,
    });

    if (shouldResume) {
      latestOutputFollowPausedByUserRef.current = false;
      latestOutputFollowResumeIntentRef.current = false;
    }
    followLatestOutputRef.current = canFollowLatestOutput ? shouldResume : false;
  }

  useLayoutEffect(() => {
    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }
    applyRunStageComposerVars(element);
  }, [applyRunStageComposerVars]);

  useLayoutEffect(() => {
    if (showCenterComposer) {
      setComposerContentBottomInsetPx(0);
      return;
    }

    const dockElement = composerDockRef.current;
    if (!dockElement) {
      return;
    }
    const measuredDockElement: HTMLDivElement = dockElement;

    function currentDockContentElement(): HTMLElement | undefined {
      const contentElement =
        measuredDockElement.querySelector<HTMLElement>(".composer-dock-stack") ??
        measuredDockElement.firstElementChild;
      return contentElement instanceof HTMLElement ? contentElement : undefined;
    }

    function syncComposerBottomInset(): void {
      const dockRect = measuredDockElement.getBoundingClientRect();
      const contentTop = currentDockContentElement()?.getBoundingClientRect().top;
      const nextInset = Math.ceil(
        composerContentBottomInset({
          dockBottom: dockRect.bottom,
          contentTop,
          fallbackInset: dockRect.height,
        }),
      );
      setComposerContentBottomInsetPx((current) =>
        current === nextInset ? current : nextInset,
      );
    }

    syncComposerBottomInset();
    const resizeObserver = new ResizeObserver(syncComposerBottomInset);
    resizeObserver.observe(measuredDockElement);
    const contentElement = currentDockContentElement();
    if (contentElement && contentElement !== measuredDockElement) {
      resizeObserver.observe(contentElement);
    }

    return () => {
      resizeObserver.disconnect();
    };
  }, [showCenterComposer]);

  useLayoutEffect(() => {
    const element = runStageScrollRef.current;
    if (!latestPromptId || !element) {
      pendingPromptAnchorRef.current = false;
      setLatestTurnMinHeightPx((current) => current === 0 ? current : 0);
      return;
    }

    const turn = findTurnElement(element, latestPromptId);
    if (!turn) {
      setLatestTurnMinHeightPx((current) => current === 0 ? current : 0);
      return;
    }

    const shouldAnchorPrompt = pendingPromptAnchorRef.current;
    const bottomReserve = latestTurnMinHeightBottomReserve({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending: shouldAnchorPrompt,
      outputGap: composerOutputGapPx,
    });
    const promptAnchorSlack = latestTurnPromptAnchorSlack({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending: shouldAnchorPrompt,
      outputGap: composerOutputGapPx,
    });
    const nextMinHeight = Math.ceil(
      anchoredTurnMinHeight({
        containerHeight: element.clientHeight,
        anchorOffset: promptAnchorOffset(element),
        bottomReserve,
      }) + promptAnchorSlack,
    );
    if (nextMinHeight !== latestTurnMinHeightPx) {
      setLatestTurnMinHeightPx(nextMinHeight);
      return;
    }

    if (shouldAnchorPrompt) {
      pendingPromptAnchorRef.current = false;
      scrollPromptToAnchor(latestPromptId);
      return;
    }

    if (followLatestOutputRef.current && !latestOutputFollowPausedByUserRef.current) {
      scrollLatestTurnToScrollTail(latestPromptId);
    }
  }, [
    composerOutputGapPx,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeItems,
  ]);

  useLayoutEffect(() => {
    if (!latestPromptId) {
      return;
    }

    const element = runStageScrollRef.current;
    if (!element) {
      return;
    }

    const turn = findTurnElement(element, latestPromptId);
    if (!turn) {
      return;
    }

    const resizeObserver = new ResizeObserver(() => {
      measureLatestTurnMinHeight(latestPromptId);
    });
    resizeObserver.observe(element);
    resizeObserver.observe(turn);

    return () => {
      resizeObserver.disconnect();
    };
  }, [composerOutputGapPx, latestPromptId]);

  useEffect(() => {
    return () => {
      runStageInputIntentCleanupRef.current?.();
      runStageInputIntentCleanupRef.current = null;
      runStageResizeObserverRef.current?.disconnect();
      runStageResizeObserverRef.current = null;
    };
  }, []);

  return {
    activeWorkbenchTurnId,
    composerContentInsetPx,
    composerDockRef,
    composerSafeAreaPx,
    handleRunStageScroll,
    handleTurnMarkerClick,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeSections,
    prepareForNewTask,
    prepareForRunStart,
    prepareForThreadSwitch,
    runStageViewportHeightPx,
    setRunStageScrollElement,
  };
}
