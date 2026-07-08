import type { RunNarrativeItem } from "./run-narrative";

type PromptNarrativeItem = Extract<RunNarrativeItem, { kind: "prompt" }>;

export type NarrativeTurnSection = {
  id: string;
  prompt: PromptNarrativeItem;
  items: RunNarrativeItem[];
};

export type NarrativeTurnSections = {
  leadItems: RunNarrativeItem[];
  turns: NarrativeTurnSection[];
};

export type NarrativeTurnBodySegment =
  | {
      kind: "items";
      id: string;
      items: RunNarrativeItem[];
    }
  | {
      kind: "executionProcess";
      id: string;
      title: string;
      items: RunNarrativeItem[];
      open: boolean;
    };

export type TurnAnchorPosition = {
  id: string;
  top: number;
};

export type TurnMarkerRailMode = "hidden" | "regular" | "compact" | "summary";

export type TurnMarkerRailItem = {
  id: string;
  index: number;
};

export type TurnMarkerRailDisplay = {
  mode: TurnMarkerRailMode;
  items: TurnMarkerRailItem[];
};

export type LatestOutputFollowMode =
  | "idle"
  | "anchoring"
  | "following_latest"
  | "paused_by_user";

export type LatestOutputResumeIntent =
  | "none"
  | "wheel_down"
  | "touch_to_latest"
  | "scroll_to_tail"
  | "return_button";

export type ProgrammaticRunStageScrollReason =
  | "none"
  | "anchor_prompt"
  | "follow_latest"
  | "marker_jump"
  | "thread_restore";

const TURN_MARKER_MIN_TURN_COUNT = 3;
const TURN_MARKER_REGULAR_LIMIT = 12;
const TURN_MARKER_COMPACT_LIMIT = 24;
const TURN_MARKER_SUMMARY_LIMIT = 24;
const EXECUTION_PROCESS_SUMMARY_TITLE = "执行过程";

const TURN_MARKER_REGULAR_SIZE = { itemHeight: 18, gap: 14 };
const TURN_MARKER_COMPACT_SIZE = { itemHeight: 12, gap: 8 };
const TURN_MARKER_SUMMARY_SIZE = { itemHeight: 8, gap: 5 };

export function diffLineStats(diff?: string): { added: number; removed: number } {
  if (!diff) {
    return { added: 0, removed: 0 };
  }

  return diff.split("\n").reduce(
    (stats, line) => {
      if (line.startsWith("+") && !line.startsWith("+++")) {
        stats.added += 1;
      }
      if (line.startsWith("-") && !line.startsWith("---")) {
        stats.removed += 1;
      }
      return stats;
    },
    { added: 0, removed: 0 },
  );
}

export function shouldUseCenterComposer({
  hasNarrative,
  hasRun,
}: {
  hasNarrative: boolean;
  hasRun: boolean;
  hasNotice: boolean;
  hasError: boolean;
  needsRestart: boolean;
}): boolean {
  return !hasNarrative && !hasRun;
}

export function shouldShowAssistantAnswerFooter({
  status,
  isFinal,
}: {
  status: Extract<RunNarrativeItem, { kind: "answer" }>["status"];
  isFinal?: boolean;
}): boolean {
  return status === "done" && isFinal === true;
}

export function shouldShowTurnMarkerRail({
  showCenterComposer,
  turnCount,
}: {
  showCenterComposer: boolean;
  turnCount: number;
}): boolean {
  return !showCenterComposer && turnCount >= TURN_MARKER_MIN_TURN_COUNT;
}

function markerSetHeight({
  count,
  itemHeight,
  gap,
}: {
  count: number;
  itemHeight: number;
  gap: number;
}): number {
  if (count <= 0) {
    return 0;
  }
  return count * itemHeight + (count - 1) * gap;
}

function markerCapacityForHeight({
  height,
  itemHeight,
  gap,
}: {
  height: number;
  itemHeight: number;
  gap: number;
}): number {
  if (height <= 0) {
    return 0;
  }
  return Math.max(1, Math.floor((height + gap) / (itemHeight + gap)));
}

function turnMarkerItemsForAll(turnIds: string[]): TurnMarkerRailItem[] {
  return turnIds.map((id, index) => ({ id, index }));
}

function sampledTurnMarkerItems({
  turnIds,
  maxItems,
  activeTurnId,
  latestTurnId,
}: {
  turnIds: string[];
  maxItems: number;
  activeTurnId?: string;
  latestTurnId?: string;
}): TurnMarkerRailItem[] {
  const turnCount = turnIds.length;
  const itemCount = Math.min(Math.max(0, maxItems), turnCount);
  if (itemCount >= turnCount) {
    return turnMarkerItemsForAll(turnIds);
  }
  if (itemCount <= 0) {
    return [];
  }

  const focusIndex = Math.max(
    0,
    turnIds.findIndex((id) => id === activeTurnId || id === latestTurnId),
  );
  const indexes = new Set<number>();
  const addIndex = (index: number) => {
    if (index >= 0 && index < turnCount && indexes.size < itemCount) {
      indexes.add(index);
    }
  };

  addIndex(0);
  addIndex(focusIndex);
  addIndex(turnCount - 1);
  addIndex(focusIndex - 1);
  addIndex(focusIndex + 1);

  for (let step = 1; indexes.size < itemCount && step < itemCount; step += 1) {
    addIndex(Math.round((step * (turnCount - 1)) / (itemCount - 1)));
  }

  for (let index = 0; indexes.size < itemCount && index < turnCount; index += 1) {
    addIndex(index);
  }

  return Array.from(indexes)
    .sort((a, b) => a - b)
    .map((index) => ({ id: turnIds[index], index }));
}

export function turnMarkerRailDisplay({
  showCenterComposer,
  turnIds,
  activeTurnId,
  latestTurnId,
  railHeight,
}: {
  showCenterComposer: boolean;
  turnIds: string[];
  activeTurnId?: string;
  latestTurnId?: string;
  railHeight: number;
}): TurnMarkerRailDisplay {
  const cleanTurnIds = turnIds.filter((id) => id.length > 0);
  const turnCount = cleanTurnIds.length;
  if (!shouldShowTurnMarkerRail({ showCenterComposer, turnCount })) {
    return { mode: "hidden", items: [] };
  }

  const availableHeight = Math.max(0, railHeight);
  if (
    turnCount <= TURN_MARKER_REGULAR_LIMIT &&
    (
      availableHeight === 0 ||
      markerSetHeight({ count: turnCount, ...TURN_MARKER_REGULAR_SIZE }) <= availableHeight
    )
  ) {
    return { mode: "regular", items: turnMarkerItemsForAll(cleanTurnIds) };
  }

  if (
    turnCount <= TURN_MARKER_COMPACT_LIMIT &&
    (
      availableHeight === 0 ||
      markerSetHeight({ count: turnCount, ...TURN_MARKER_COMPACT_SIZE }) <= availableHeight
    )
  ) {
    return { mode: "compact", items: turnMarkerItemsForAll(cleanTurnIds) };
  }

  const summaryCapacity = availableHeight === 0
    ? TURN_MARKER_SUMMARY_LIMIT
    : Math.min(
      TURN_MARKER_SUMMARY_LIMIT,
      markerCapacityForHeight({ height: availableHeight, ...TURN_MARKER_SUMMARY_SIZE }),
    );
  if (summaryCapacity < TURN_MARKER_MIN_TURN_COUNT) {
    return { mode: "hidden", items: [] };
  }

  return {
    mode: "summary",
    items: sampledTurnMarkerItems({
      turnIds: cleanTurnIds,
      maxItems: summaryCapacity,
      activeTurnId,
      latestTurnId,
    }),
  };
}

export function turnMarkerRailHeight({
  scrollViewportHeight,
  composerContentInset,
  bottomSafeArea,
  topInset = 28,
  minimumBottom = 28,
}: {
  scrollViewportHeight: number;
  composerContentInset: number;
  bottomSafeArea: number;
  topInset?: number;
  minimumBottom?: number;
}): number {
  const stageHeight =
    Math.max(0, scrollViewportHeight) + Math.max(0, composerContentInset);
  return Math.max(
    0,
    stageHeight -
      Math.max(0, topInset) -
      Math.max(Math.max(0, minimumBottom), bottomSafeArea),
  );
}

export function groupNarrativeByTurns(
  items: RunNarrativeItem[],
): NarrativeTurnSections {
  const leadItems: RunNarrativeItem[] = [];
  const turns: NarrativeTurnSection[] = [];
  let activeTurn: NarrativeTurnSection | null = null;

  for (const item of items) {
    if (item.kind === "prompt") {
      activeTurn = {
        id: item.id,
        prompt: item,
        items: [],
      };
      turns.push(activeTurn);
      continue;
    }

    if (!activeTurn) {
      leadItems.push(item);
      continue;
    }

    activeTurn.items.push(item);
  }

  return { leadItems, turns };
}

export function projectNarrativeTurnBodySegments({
  turnId,
  items,
  executionProcessOpen,
}: {
  turnId: string;
  items: RunNarrativeItem[];
  executionProcessOpen: boolean;
}): NarrativeTurnBodySegment[] {
  if (items.length === 0) {
    return [];
  }

  const firstAnswerIndex = items.findIndex(
    (item) => item.kind === "answer" && item.isFinal === true,
  );
  if (firstAnswerIndex <= 0) {
    return [
      {
        kind: "items",
        id: `turn-items:${turnId}`,
        items,
      },
    ];
  }

  const executionProcessItems = items.slice(0, firstAnswerIndex);
  const answerAndAfterItems = items.slice(firstAnswerIndex);
  const segments: NarrativeTurnBodySegment[] = [
    {
      kind: "executionProcess",
      id: `execution-process:${turnId}`,
      title: EXECUTION_PROCESS_SUMMARY_TITLE,
      items: executionProcessItems,
      open: executionProcessOpen,
    },
  ];

  if (answerAndAfterItems.length > 0) {
    segments.push({
      kind: "items",
      id: `answer-and-after:${turnId}`,
      items: answerAndAfterItems,
    });
  }

  return segments;
}

export function scrollTopForPromptAnchor({
  currentScrollTop,
  containerTop,
  promptTop,
  anchorOffset,
}: {
  currentScrollTop: number;
  containerTop: number;
  promptTop: number;
  anchorOffset: number;
}): number {
  return Math.max(0, currentScrollTop + promptTop - containerTop - anchorOffset);
}

export function activeTurnIdForAnchor({
  turns,
  anchorY,
}: {
  turns: TurnAnchorPosition[];
  anchorY: number;
}): string | undefined {
  let activeId = turns[0]?.id;
  for (const turn of turns) {
    if (turn.top > anchorY) {
      break;
    }
    activeId = turn.id;
  }
  return activeId;
}

export function composerBottomSafeArea({
  composerHeight,
  safeGap,
}: {
  composerHeight: number;
  safeGap: number;
}): number {
  return Math.max(0, composerHeight) + Math.max(0, safeGap);
}

export function composerContentBottomInset({
  dockBottom,
  contentTop,
  fallbackInset = 0,
}: {
  dockBottom: number;
  contentTop?: number;
  fallbackInset?: number;
}): number {
  if (typeof contentTop !== "number" || !Number.isFinite(contentTop)) {
    return Math.max(0, fallbackInset);
  }
  return Math.max(0, dockBottom - contentTop);
}

export function anchoredTurnMinHeight({
  containerHeight,
  anchorOffset,
  bottomReserve = 0,
}: {
  containerHeight: number;
  anchorOffset: number;
  bottomReserve?: number;
}): number {
  return Math.max(0, containerHeight - anchorOffset - Math.max(0, bottomReserve));
}

export function latestTurnMinHeightBottomReserve({
  hasBodyItems: _hasBodyItems,
  isPromptAnchorPending: _isPromptAnchorPending,
  outputGap,
}: {
  hasBodyItems: boolean;
  isPromptAnchorPending: boolean;
  outputGap: number;
}): number {
  return Math.max(0, outputGap);
}

export function latestTurnPromptAnchorSlack({
  hasBodyItems: _hasBodyItems,
  isPromptAnchorPending: _isPromptAnchorPending,
  outputGap: _outputGap,
}: {
  hasBodyItems: boolean;
  isPromptAnchorPending: boolean;
  outputGap: number;
}): number {
  return 0;
}

export function scrollTopForLatestTurnScrollTail({
  maxScrollTop,
}: {
  maxScrollTop: number;
}): number {
  return Math.max(0, maxScrollTop);
}

export function shouldFollowLatestTurnScrollTail({
  hasBodyItems,
  isPromptAnchorPending,
}: {
  hasBodyItems: boolean;
  isPromptAnchorPending: boolean;
}): boolean {
  return hasBodyItems && !isPromptAnchorPending;
}

export function shouldCancelPendingPromptAnchorForScrollIntent({
  hasDirectUserInput,
}: {
  hasDirectUserInput: boolean;
}): boolean {
  return hasDirectUserInput;
}

export function isNearLatestTurnOutputTail({
  containerTop,
  outputBottom,
  containerHeight,
  bottomReserve,
  threshold,
}: {
  containerTop: number;
  outputBottom: number;
  containerHeight: number;
  bottomReserve: number;
  threshold: number;
}): boolean {
  const visibleHeight = Math.max(1, containerHeight - Math.max(0, bottomReserve));
  return outputBottom - containerTop <= visibleHeight + Math.max(0, threshold);
}

export function isNearScrollBottom({
  scrollTop,
  scrollHeight,
  clientHeight,
  threshold,
}: {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
  threshold: number;
}): boolean {
  return scrollHeight - scrollTop - clientHeight <= Math.max(0, threshold);
}

export function shouldPauseLatestOutputFollowForWheel({
  deltaY,
}: {
  deltaY: number;
}): boolean {
  return deltaY < -1;
}

export function shouldPauseLatestOutputFollowForScroll({
  previousScrollTop,
  nextScrollTop,
  threshold = 1,
}: {
  previousScrollTop: number | null;
  nextScrollTop: number;
  threshold?: number;
}): boolean {
  if (previousScrollTop === null || !Number.isFinite(previousScrollTop)) {
    return false;
  }
  return nextScrollTop < previousScrollTop - Math.max(0, threshold);
}

export function shouldRequestLatestOutputFollowResumeForWheel({
  deltaY,
}: {
  deltaY: number;
}): boolean {
  return deltaY > 1;
}

export function shouldPauseLatestOutputFollowForTouchMove({
  startY,
  currentY,
  threshold = 4,
}: {
  startY: number;
  currentY: number;
  threshold?: number;
}): boolean {
  return currentY - startY > Math.max(0, threshold);
}

export function shouldRequestLatestOutputFollowResumeForTouchMove({
  startY,
  currentY,
  threshold = 4,
}: {
  startY: number;
  currentY: number;
  threshold?: number;
}): boolean {
  return startY - currentY > Math.max(0, threshold);
}

export function shouldResumeLatestOutputFollow({
  isPausedByUser,
  isNearTail,
  isAtScrollBottom,
  hasResumeIntent,
}: {
  isPausedByUser: boolean;
  isNearTail: boolean;
  isAtScrollBottom: boolean;
  hasResumeIntent: boolean;
}): boolean {
  if (!isNearTail) {
    return false;
  }
  if (!isPausedByUser) {
    return true;
  }
  return hasResumeIntent && isAtScrollBottom;
}

export function shouldShowReturnToLatestAffordance({
  followMode,
  hasLatestTurn,
}: {
  followMode: LatestOutputFollowMode;
  hasLatestTurn: boolean;
}): boolean {
  return followMode === "paused_by_user" && hasLatestTurn;
}
