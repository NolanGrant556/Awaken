import { useCallback, useEffect, useLayoutEffect, useMemo, useReducer, useRef, useState } from "react";
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
  shouldFollowLatestTurnScrollTail,
  shouldPauseLatestOutputFollowForScroll,
  shouldPauseLatestOutputFollowForTouchMove,
  shouldPauseLatestOutputFollowForWheel,
  shouldRequestLatestOutputFollowResumeForTouchMove,
  shouldRequestLatestOutputFollowResumeForWheel,
  shouldResumeLatestOutputFollow,
  shouldShowReturnToLatestAffordance,
  type LatestOutputFollowMode,
  type LatestOutputResumeIntent,
  type ProgrammaticRunStageScrollReason,
} from "./workbenchPresentation";

const DEFAULT_PROMPT_ANCHOR_OFFSET = 0;
const RUN_STAGE_COMPOSER_SAFE_GAP = 72;
const LATEST_OUTPUT_FOLLOW_THRESHOLD = 48;
const RETURN_TO_LATEST_BUTTON_GAP_PX = 12;

type RunStageScrollControllerOptions = {
  showCenterComposer: boolean;
  narrativeItems: RunNarrativeItem[];
};

type ScrollInteractionState = {
  currentTurnId?: string;
  pendingPromptAnchorTurnId?: string;
  followMode: LatestOutputFollowMode;
  resumeIntent: LatestOutputResumeIntent;
  programmaticScrollReason: ProgrammaticRunStageScrollReason;
  programmaticScrollTop: number | null;
};

type ScrollInteractionAction =
  | { type: "prepare-run-start" }
  | { type: "prepare-thread-switch" }
  | { type: "prepare-new-task" }
  | { type: "bind-latest-turn"; turnId: string }
  | { type: "sync-current-turn"; turnId?: string }
  | { type: "pause-by-user"; cancelPendingPromptAnchor: boolean }
  | { type: "set-resume-intent"; intent: Exclude<LatestOutputResumeIntent, "none"> }
  | { type: "resume-follow"; turnId?: string }
  | { type: "prompt-anchored"; turnId: string; hasBodyItems: boolean }
  | { type: "latest-body-ready" }
  | { type: "start-programmatic-scroll"; reason: ProgrammaticRunStageScrollReason; scrollTop: number }
  | { type: "clear-programmatic-scroll"; scrollTop?: number }
  | { type: "reset-empty" };

const initialScrollInteractionState: ScrollInteractionState = {
  followMode: "idle",
  resumeIntent: "none",
  programmaticScrollReason: "none",
  programmaticScrollTop: null,
};

function reduceScrollInteractionState(
  state: ScrollInteractionState,
  action: ScrollInteractionAction,
): ScrollInteractionState {
  switch (action.type) {
    case "prepare-run-start":
    case "prepare-thread-switch":
      return {
        ...state,
        currentTurnId: undefined,
        pendingPromptAnchorTurnId: undefined,
        followMode: "anchoring",
        resumeIntent: "none",
        programmaticScrollReason: "none",
        programmaticScrollTop: null,
      };
    case "prepare-new-task":
    case "reset-empty":
      return initialScrollInteractionState;
    case "bind-latest-turn":
      if (!action.turnId || state.pendingPromptAnchorTurnId === action.turnId) {
        return state;
      }
      if (state.followMode !== "anchoring") {
        return state;
      }
      return {
        ...state,
        pendingPromptAnchorTurnId: action.turnId,
      };
    case "sync-current-turn":
      if (state.currentTurnId === action.turnId) {
        return state;
      }
      return {
        ...state,
        currentTurnId: action.turnId,
      };
    case "pause-by-user":
      return {
        ...state,
        pendingPromptAnchorTurnId: action.cancelPendingPromptAnchor
          ? undefined
          : state.pendingPromptAnchorTurnId,
        followMode: "paused_by_user",
        resumeIntent: "none",
      };
    case "set-resume-intent":
      if (state.resumeIntent === action.intent) {
        return state;
      }
      return {
        ...state,
        resumeIntent: action.intent,
      };
    case "resume-follow":
      return {
        ...state,
        currentTurnId: action.turnId ?? state.currentTurnId,
        pendingPromptAnchorTurnId: undefined,
        followMode: "following_latest",
        resumeIntent: "none",
      };
    case "prompt-anchored":
      return {
        ...state,
        currentTurnId: action.turnId,
        pendingPromptAnchorTurnId: undefined,
        followMode: action.hasBodyItems ? "following_latest" : "anchoring",
        resumeIntent: "none",
      };
    case "latest-body-ready":
      if (state.followMode !== "anchoring" || state.pendingPromptAnchorTurnId) {
        return state;
      }
      return {
        ...state,
        followMode: "following_latest",
      };
    case "start-programmatic-scroll":
      return {
        ...state,
        programmaticScrollReason: action.reason,
        programmaticScrollTop: action.scrollTop,
      };
    case "clear-programmatic-scroll":
      if (
        action.scrollTop !== undefined &&
        state.programmaticScrollTop !== null &&
        Math.abs(state.programmaticScrollTop - action.scrollTop) >= 2
      ) {
        return state;
      }
      if (state.programmaticScrollReason === "none" && state.programmaticScrollTop === null) {
        return state;
      }
      return {
        ...state,
        programmaticScrollReason: "none",
        programmaticScrollTop: null,
      };
    default:
      return state;
  }
}

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
  const [interactionState, dispatchInteraction] = useReducer(
    reduceScrollInteractionState,
    initialScrollInteractionState,
  );
  const interactionStateRef = useRef(interactionState);
  const runStageLastScrollTopRef = useRef<number | null>(null);
  const runStageTouchStartYRef = useRef<number | null>(null);
  const runStageInputIntentCleanupRef = useRef<(() => void) | null>(null);
  const runStageResizeObserverRef = useRef<ResizeObserver | null>(null);
  const runStageScrollRef = useRef<HTMLDivElement | null>(null);
  const composerDockElementRef = useRef<HTMLDivElement | null>(null);

  const [latestTurnMinHeightPx, setLatestTurnMinHeightPx] = useState(0);
  const [composerContentBottomInsetPx, setComposerContentBottomInsetPx] = useState(0);
  const [composerBarBottomInsetPx, setComposerBarBottomInsetPx] = useState(0);
  const [runStageViewportHeightPx, setRunStageViewportHeightPx] = useState(0);
  const [runStageAttachVersion, setRunStageAttachVersion] = useState(0);
  const [composerDockAttachVersion, setComposerDockAttachVersion] = useState(0);

  const narrativeSections = useMemo(
    () => groupNarrativeByTurns(narrativeItems),
    [narrativeItems],
  );
  const latestTurn = narrativeSections.turns[narrativeSections.turns.length - 1];
  const latestPromptId = latestTurn ? latestTurn.id : undefined;
  const hasPromptNarrative = narrativeSections.turns.length > 0;
  const composerContentInsetPx = hasPromptNarrative ? composerContentBottomInsetPx : 0;
  const composerOutputGapPx = hasPromptNarrative ? RUN_STAGE_COMPOSER_SAFE_GAP : 0;
  const composerBarInsetPx = hasPromptNarrative ? composerBarBottomInsetPx : 0;
  const composerSafeAreaPx = composerBottomSafeArea({
    composerHeight: composerContentInsetPx,
    safeGap: composerOutputGapPx,
  });
  const composerBarSafeAreaPx = composerBottomSafeArea({
    composerHeight: composerBarInsetPx,
    safeGap: composerOutputGapPx,
  });
  const activeWorkbenchTurnId =
    interactionState.currentTurnId &&
    narrativeSections.turns.some((turn) => turn.id === interactionState.currentTurnId)
      ? interactionState.currentTurnId
      : latestPromptId;
  const returnToLatestVisible = shouldShowReturnToLatestAffordance({
    followMode: interactionState.followMode,
    hasLatestTurn: Boolean(latestPromptId),
  });
  const returnToLatestBottomPx = Math.max(16, composerBarBottomInsetPx + RETURN_TO_LATEST_BUTTON_GAP_PX);

  useLayoutEffect(() => {
    interactionStateRef.current = interactionState;
  }, [interactionState]);

  const setComposerDockElement = useCallback((element: HTMLDivElement | null) => {
    composerDockElementRef.current = element;
    if (element) {
      setComposerDockAttachVersion((current) => current + 1);
    }
  }, []);

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

  const prepareForRunStart = useCallback((): void => {
    dispatchInteraction({ type: "prepare-run-start" });
    setLatestTurnMinHeightPx(0);
  }, []);

  const prepareForThreadSwitch = useCallback((): void => {
    dispatchInteraction({ type: "prepare-thread-switch" });
    setLatestTurnMinHeightPx(0);
  }, []);

  const prepareForNewTask = useCallback((): void => {
    dispatchInteraction({ type: "prepare-new-task" });
    setLatestTurnMinHeightPx(0);
  }, []);

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
    dispatchInteraction({ type: "sync-current-turn", turnId: nextActiveId });
  }

  function pauseLatestOutputFollowForUser(cancelPendingPromptAnchor: boolean): void {
    dispatchInteraction({ type: "pause-by-user", cancelPendingPromptAnchor });
  }

  function measureLatestTurnMinHeight(promptId: string): void {
    const element = runStageScrollRef.current;
    const turn = element ? findTurnElement(element, promptId) : null;
    if (!element || !turn) {
      setLatestTurnMinHeightPx(0);
      return;
    }

    const isPromptAnchorPending = interactionStateRef.current.pendingPromptAnchorTurnId === promptId;
    const bottomReserve = latestTurnMinHeightBottomReserve({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending,
      outputGap: composerOutputGapPx,
    });
    const promptAnchorSlack = latestTurnPromptAnchorSlack({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending,
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

  function scrollRunStageTo(
    element: HTMLElement,
    scrollTop: number,
    reason: ProgrammaticRunStageScrollReason,
  ): void {
    const maxScrollTop = Math.max(0, element.scrollHeight - element.clientHeight);
    const nextScrollTop = Math.min(maxScrollTop, Math.max(0, scrollTop));
    dispatchInteraction({
      type: "start-programmatic-scroll",
      reason,
      scrollTop: nextScrollTop,
    });
    runStageLastScrollTopRef.current = nextScrollTop;
    element.scrollTo({
      top: nextScrollTop,
      behavior: "auto",
    });
    window.requestAnimationFrame(() => {
      dispatchInteraction({ type: "clear-programmatic-scroll", scrollTop: nextScrollTop });
    });
  }

  function scrollPromptToAnchor(
    promptId: string,
    reason: ProgrammaticRunStageScrollReason = "anchor_prompt",
  ): boolean {
    const element = runStageScrollRef.current;
    if (!element) {
      return false;
    }

    const turn = findTurnElement(element, promptId);
    if (!turn) {
      return false;
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
      reason,
    );
    return true;
  }

  function scrollLatestTurnToScrollTail(
    promptId: string,
    reason: ProgrammaticRunStageScrollReason = "follow_latest",
  ): boolean {
    const element = runStageScrollRef.current;
    if (!element) {
      return false;
    }

    const turn = findTurnElement(element, promptId);
    if (!turn) {
      return false;
    }

    const isPromptAnchorPending = interactionStateRef.current.pendingPromptAnchorTurnId === promptId;
    if (
      !shouldFollowLatestTurnScrollTail({
        hasBodyItems: turnHasBodyItems(turn),
        isPromptAnchorPending,
      })
    ) {
      return false;
    }

    const nextScrollTop = scrollTopForLatestTurnScrollTail({
      maxScrollTop: Math.max(0, element.scrollHeight - element.clientHeight),
    });

    if (Math.abs(nextScrollTop - element.scrollTop) < 1) {
      return true;
    }

    scrollRunStageTo(element, nextScrollTop, reason);
    return true;
  }

  const setRunStageScrollElement = useCallback((element: HTMLDivElement | null) => {
    runStageInputIntentCleanupRef.current?.();
    runStageInputIntentCleanupRef.current = null;
    runStageResizeObserverRef.current?.disconnect();
    runStageResizeObserverRef.current = null;
    runStageScrollRef.current = element;
    if (!element) {
      runStageLastScrollTopRef.current = null;
      setRunStageViewportHeightPx((current) => current === 0 ? current : 0);
      return;
    }
    runStageLastScrollTopRef.current = element.scrollTop;
    setRunStageAttachVersion((current) => current + 1);
    applyRunStageComposerVars(element);

    const syncRunStageViewportHeight = () => {
      const nextHeight = element.clientHeight;
      setRunStageViewportHeightPx((current) => current === nextHeight ? current : nextHeight);
    };
    syncRunStageViewportHeight();
    syncActiveTurnFromScroll(element);
    if (typeof ResizeObserver !== "undefined") {
      const resizeObserver = new ResizeObserver(syncRunStageViewportHeight);
      resizeObserver.observe(element);
      runStageResizeObserverRef.current = resizeObserver;
    }

    const handleWheel = (event: WheelEvent) => {
      if (shouldPauseLatestOutputFollowForWheel({ deltaY: event.deltaY })) {
        pauseLatestOutputFollowForUser(true);
        return;
      }
      if (shouldRequestLatestOutputFollowResumeForWheel({ deltaY: event.deltaY })) {
        dispatchInteraction({ type: "set-resume-intent", intent: "wheel_down" });
      }
    };
    const handlePointerDown = () => {
      pauseLatestOutputFollowForUser(true);
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
        pauseLatestOutputFollowForUser(true);
        return;
      }
      if (
        startY !== null &&
        currentY !== undefined &&
        shouldRequestLatestOutputFollowResumeForTouchMove({ startY, currentY })
      ) {
        dispatchInteraction({ type: "set-resume-intent", intent: "touch_to_latest" });
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
    pauseLatestOutputFollowForUser(true);
    dispatchInteraction({ type: "sync-current-turn", turnId });
    scrollPromptToAnchor(turnId, "marker_jump");
  }

  function handleReturnToLatestClick(): void {
    if (!latestPromptId) {
      return;
    }

    const element = runStageScrollRef.current;
    const turn = element ? findTurnElement(element, latestPromptId) : null;
    if (!element || !turn) {
      return;
    }

    dispatchInteraction({ type: "set-resume-intent", intent: "return_button" });
    if (
      interactionStateRef.current.pendingPromptAnchorTurnId === latestPromptId ||
      !turnHasBodyItems(turn)
    ) {
      if (scrollPromptToAnchor(latestPromptId, "anchor_prompt")) {
        dispatchInteraction({
          type: "prompt-anchored",
          turnId: latestPromptId,
          hasBodyItems: turnHasBodyItems(turn),
        });
      }
      return;
    }

    if (scrollLatestTurnToScrollTail(latestPromptId, "follow_latest")) {
      dispatchInteraction({ type: "resume-follow", turnId: latestPromptId });
      return;
    }

    if (scrollPromptToAnchor(latestPromptId, "anchor_prompt")) {
      dispatchInteraction({ type: "resume-follow", turnId: latestPromptId });
    }
  }

  function handleRunStageScroll(event: ReactUIEvent<HTMLDivElement>): void {
    const element = event.currentTarget;
    const previousScrollTop = runStageLastScrollTopRef.current;
    const nextScrollTop = element.scrollTop;
    runStageLastScrollTopRef.current = nextScrollTop;
    const state = interactionStateRef.current;
    const programmaticScrollTop = state.programmaticScrollTop;
    const isProgrammaticScroll =
      programmaticScrollTop !== null &&
      Math.abs(nextScrollTop - programmaticScrollTop) < 2;
    if (isProgrammaticScroll) {
      dispatchInteraction({ type: "clear-programmatic-scroll", scrollTop: nextScrollTop });
    }

    syncActiveTurnFromScroll(element);

    if (isProgrammaticScroll) {
      return;
    }

    if (!latestPromptId) {
      return;
    }

    const turn = findTurnElement(element, latestPromptId);
    if (!turn) {
      return;
    }

    const isAtScrollBottom = isNearScrollBottom({
      scrollTop: nextScrollTop,
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
      threshold: 2,
    });

    if (
      shouldPauseLatestOutputFollowForScroll({
        previousScrollTop,
        nextScrollTop,
        isAtScrollBottom,
      })
    ) {
      dispatchInteraction({ type: "pause-by-user", cancelPendingPromptAnchor: false });
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

    let hasResumeIntent = state.resumeIntent !== "none";
    if (state.followMode === "paused_by_user" && isNearTail && isAtScrollBottom && !hasResumeIntent) {
      dispatchInteraction({ type: "set-resume-intent", intent: "scroll_to_tail" });
      hasResumeIntent = true;
    }

    const shouldResume = shouldResumeLatestOutputFollow({
      isPausedByUser: state.followMode === "paused_by_user",
      isNearTail,
      isAtScrollBottom,
      hasResumeIntent,
    });

    if (shouldResume) {
      dispatchInteraction({ type: "resume-follow", turnId: latestPromptId });
    }
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
      setComposerBarBottomInsetPx(0);
      return;
    }

    const dockElement = composerDockElementRef.current;
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

    function currentComposerBarElement(): HTMLElement | undefined {
      const composerBarElement = measuredDockElement.querySelector<HTMLElement>(".composer-bar");
      return composerBarElement instanceof HTMLElement ? composerBarElement : undefined;
    }

    function syncComposerBottomInset(): void {
      const dockRect = measuredDockElement.getBoundingClientRect();
      const contentTop = currentDockContentElement()?.getBoundingClientRect().top;
      const composerBarTop = currentComposerBarElement()?.getBoundingClientRect().top;
      const nextInset = Math.ceil(
        composerContentBottomInset({
          dockBottom: dockRect.bottom,
          contentTop,
          fallbackInset: dockRect.height,
        }),
      );
      const nextComposerBarInset = Math.ceil(
        composerContentBottomInset({
          dockBottom: dockRect.bottom,
          contentTop: composerBarTop,
          fallbackInset: nextInset,
        }),
      );
      setComposerContentBottomInsetPx((current) =>
        current === nextInset ? current : nextInset,
      );
      setComposerBarBottomInsetPx((current) =>
        current === nextComposerBarInset ? current : nextComposerBarInset,
      );
    }

    syncComposerBottomInset();
    const resizeObserver = new ResizeObserver(syncComposerBottomInset);
    resizeObserver.observe(measuredDockElement);
    const contentElement = currentDockContentElement();
    if (contentElement && contentElement !== measuredDockElement) {
      resizeObserver.observe(contentElement);
    }
    const composerBarElement = currentComposerBarElement();
    if (composerBarElement && composerBarElement !== measuredDockElement) {
      resizeObserver.observe(composerBarElement);
    }

    return () => {
      resizeObserver.disconnect();
    };
  }, [composerDockAttachVersion, showCenterComposer]);

  useLayoutEffect(() => {
    const element = runStageScrollRef.current;
    if (!latestPromptId) {
      dispatchInteraction({ type: "reset-empty" });
      setLatestTurnMinHeightPx((current) => current === 0 ? current : 0);
      return;
    }
    if (!element) {
      return;
    }

    if (
      interactionState.followMode === "anchoring" &&
      !interactionState.pendingPromptAnchorTurnId &&
      interactionState.currentTurnId !== latestPromptId
    ) {
      dispatchInteraction({ type: "bind-latest-turn", turnId: latestPromptId });
      return;
    }

    const turn = findTurnElement(element, latestPromptId);
    if (!turn) {
      setLatestTurnMinHeightPx((current) => current === 0 ? current : 0);
      return;
    }

    const isPromptAnchorPending =
      interactionState.pendingPromptAnchorTurnId === latestPromptId;
    const bottomReserve = latestTurnMinHeightBottomReserve({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending,
      outputGap: composerOutputGapPx,
    });
    const promptAnchorSlack = latestTurnPromptAnchorSlack({
      hasBodyItems: turnHasBodyItems(turn),
      isPromptAnchorPending,
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

    if (isPromptAnchorPending) {
      if (scrollPromptToAnchor(latestPromptId, "anchor_prompt")) {
        dispatchInteraction({
          type: "prompt-anchored",
          turnId: latestPromptId,
          hasBodyItems: turnHasBodyItems(turn),
        });
      }
      return;
    }

    if (interactionState.followMode === "anchoring" && turnHasBodyItems(turn)) {
      dispatchInteraction({ type: "latest-body-ready" });
      return;
    }

    if (interactionState.followMode === "following_latest") {
      scrollLatestTurnToScrollTail(latestPromptId, "follow_latest");
    }
  }, [
    composerOutputGapPx,
    interactionState.currentTurnId,
    interactionState.followMode,
    interactionState.pendingPromptAnchorTurnId,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeItems,
    runStageAttachVersion,
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
      // 异步高度变化（mermaid 占位坍缩、SVG/图片注入）发生时，following 状态下
      // 在同一帧 re-pin 到 native tail；RO 回调先于 paint，校正不可见。
      // 暂停、pending anchor 和非 follow 的程序滚动期间不介入，避免与用户意图
      // 或进行中的 anchor/marker 滚动冲突。
      const state = interactionStateRef.current;
      const hasConflictingProgrammaticScroll =
        state.programmaticScrollReason !== "none" &&
        state.programmaticScrollReason !== "follow_latest";
      if (
        state.followMode === "following_latest" &&
        !state.pendingPromptAnchorTurnId &&
        !hasConflictingProgrammaticScroll
      ) {
        scrollLatestTurnToScrollTail(latestPromptId, "follow_latest");
      }
    });
    resizeObserver.observe(element);
    resizeObserver.observe(turn);

    return () => {
      resizeObserver.disconnect();
    };
  }, [composerOutputGapPx, latestPromptId, runStageAttachVersion]);

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
    composerBarSafeAreaPx,
    composerContentInsetPx,
    setComposerDockElement,
    composerSafeAreaPx,
    handleReturnToLatestClick,
    handleRunStageScroll,
    handleTurnMarkerClick,
    latestPromptId,
    latestTurnMinHeightPx,
    narrativeSections,
    prepareForNewTask,
    prepareForRunStart,
    prepareForThreadSwitch,
    returnToLatestBottomPx,
    returnToLatestVisible,
    runStageViewportHeightPx,
    setRunStageScrollElement,
  };
}
