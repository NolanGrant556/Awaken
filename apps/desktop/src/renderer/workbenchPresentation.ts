import type { RunNarrativeItem } from "./run-narrative";

type PromptNarrativeItem = Extract<RunNarrativeItem, { kind: "prompt" }>;
type ActivityNarrativeItem = Extract<
  RunNarrativeItem,
  { kind: "exploring" | "tool" | "skill" | "hook" }
>;

export type NarrativeActivityBatch = {
  kind: "activityBatch";
  id: string;
  items: ActivityNarrativeItem[];
  actionCount: number;
  failedCount: number;
  status: "active" | "done";
};

export type NarrativeActivitySegment =
  | {
      kind: "item";
      id: string;
      item: RunNarrativeItem;
    }
  | NarrativeActivityBatch;

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
      elapsedMs?: number;
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
  "idle" | "anchoring" | "following_latest" | "paused_by_user";

export type LatestOutputResumeIntent =
  | "none"
  | "wheel_down"
  | "touch_to_latest"
  | "scroll_to_tail"
  | "return_button";

export type ProgrammaticRunStageScrollReason =
  "none" | "anchor_prompt" | "follow_latest" | "marker_jump" | "thread_restore";

const TURN_MARKER_MIN_TURN_COUNT = 3;
const TURN_MARKER_REGULAR_LIMIT = 12;
const TURN_MARKER_COMPACT_LIMIT = 24;
const TURN_MARKER_SUMMARY_LIMIT = 24;
export function executionProcessSummaryTitle(): string {
  return "执行过程";
}

export function narrativeTurnElapsedMs(items: RunNarrativeItem[]): number | undefined {
  for (let index = items.length - 1; index >= 0; index -= 1) {
    const item = items[index];
    if (
      item.kind === "answer" &&
      item.isFinal === true &&
      typeof item.elapsedMs === "number" &&
      Number.isFinite(item.elapsedMs) &&
      item.elapsedMs >= 0
    ) {
      return item.elapsedMs;
    }
  }
  return undefined;
}

const TURN_MARKER_REGULAR_SIZE = { itemHeight: 18, gap: 14 };
const TURN_MARKER_COMPACT_SIZE = { itemHeight: 12, gap: 8 };
const TURN_MARKER_SUMMARY_SIZE = { itemHeight: 8, gap: 5 };

function isActivityNarrativeItem(item: RunNarrativeItem): item is ActivityNarrativeItem {
  return item.kind === "exploring" || item.kind === "tool" || item.kind === "skill" || item.kind === "hook";
}

function activityActionCount(item: ActivityNarrativeItem): number {
  return item.kind === "exploring" ? item.actions.length : 1;
}

function activityFailedCount(item: ActivityNarrativeItem): number {
  if (item.kind === "exploring") {
    return item.actions.filter(
      (action) => action.status !== "active" && action.status !== "done",
    ).length;
  }
  return item.status !== "active" && item.status !== "done" ? 1 : 0;
}

function activityIsActive(item: ActivityNarrativeItem): boolean {
  if (item.status === "active") {
    return true;
  }
  return item.kind === "exploring" && item.actions.some((action) => action.status === "active");
}

/**
 * Groups only adjacent tool activity. Any user-visible narrative item is a hard boundary,
 * so this presentation-only projection cannot change runtime ordering or replay semantics.
 */
export function projectNarrativeActivitySegments({
  turnId,
  items,
}: {
  turnId: string;
  items: RunNarrativeItem[];
}): NarrativeActivitySegment[] {
  const segments: NarrativeActivitySegment[] = [];
  let pending: ActivityNarrativeItem[] = [];

  const flush = () => {
    if (pending.length === 0) return;

    const actionCount = pending.reduce((count, item) => count + activityActionCount(item), 0);
    if (actionCount <= 1) {
      for (const item of pending) {
        segments.push({ kind: "item", id: `activity-item:${turnId}:${item.id}`, item });
      }
      pending = [];
      return;
    }

    const firstItem = pending[0];
    segments.push({
      kind: "activityBatch",
      id: `activity-batch:${turnId}:${firstItem.id}`,
      items: pending,
      actionCount,
      failedCount: pending.reduce((count, item) => count + activityFailedCount(item), 0),
      status: pending.some(activityIsActive) ? "active" : "done",
    });
    pending = [];
  };

  for (const item of items) {
    if (isActivityNarrativeItem(item)) {
      pending.push(item);
      continue;
    }
    flush();
    segments.push({ kind: "item", id: `narrative-item:${turnId}:${item.id}`, item });
  }
  flush();

  return segments;
}

export function diffLineStats(diff?: string): {
  added: number;
  removed: number;
} {
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
  holdWorkbench = false,
}: {
  hasNarrative: boolean;
  hasRun: boolean;
  hasNotice: boolean;
  hasError: boolean;
  needsRestart: boolean;
  holdWorkbench?: boolean;
}): boolean {
  if (holdWorkbench) {
    return false;
  }
  return !hasNarrative && !hasRun;
}

export function workbenchHasRun({
  runId,
  isBusy,
}: {
  runId?: string;
  isBusy: boolean;
}): boolean {
  return Boolean(runId) || isBusy;
}

export function projectVisibleNarrativeItems({
  items,
  isBusy,
}: {
  items: RunNarrativeItem[];
  isBusy: boolean;
}): RunNarrativeItem[] {
  const trailingIndex = items.length - 1;
  return items.filter(
    (item, index) =>
      item.kind !== "compactBoundary" &&
      (item.kind !== "planning" || (isBusy && index === trailingIndex)),
  );
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

  for (
    let index = 0;
    indexes.size < itemCount && index < turnCount;
    index += 1
  ) {
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
    (availableHeight === 0 ||
      markerSetHeight({ count: turnCount, ...TURN_MARKER_REGULAR_SIZE }) <=
        availableHeight)
  ) {
    return { mode: "regular", items: turnMarkerItemsForAll(cleanTurnIds) };
  }

  if (
    turnCount <= TURN_MARKER_COMPACT_LIMIT &&
    (availableHeight === 0 ||
      markerSetHeight({ count: turnCount, ...TURN_MARKER_COMPACT_SIZE }) <=
        availableHeight)
  ) {
    return { mode: "compact", items: turnMarkerItemsForAll(cleanTurnIds) };
  }

  const summaryCapacity =
    availableHeight === 0
      ? TURN_MARKER_SUMMARY_LIMIT
      : Math.min(
          TURN_MARKER_SUMMARY_LIMIT,
          markerCapacityForHeight({
            height: availableHeight,
            ...TURN_MARKER_SUMMARY_SIZE,
          }),
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

export function contextUsagePanelBottomOffset({
  composerBarInset,
  gap = 12,
  minimum = 28,
}: {
  composerBarInset: number;
  gap?: number;
  minimum?: number;
}): number {
  return Math.max(
    Math.max(0, minimum),
    Math.max(0, composerBarInset) + Math.max(0, gap),
  );
}

export function subagentDetailPanelBottomOffset({
  composerContentInset,
  gap = 12,
  minimum = 28,
}: {
  composerContentInset: number;
  gap?: number;
  minimum?: number;
}): number {
  return Math.max(
    Math.max(0, minimum),
    Math.max(0, composerContentInset) + Math.max(0, gap),
  );
}

/**
 * Bottom offset of the run-plan overlay panel (contract 运行计划与任务进度
 * 设计契约 §5.3): the panel sits 12px above the measured composer content
 * inset, same rule as the subagent detail overlay.
 */
export function runPlanPanelBottomOffset({
  composerContentInset,
  gap = 12,
  minimum = 28,
}: {
  composerContentInset: number;
  gap?: number;
  minimum?: number;
}): number {
  return Math.max(
    Math.max(0, minimum),
    Math.max(0, composerContentInset) + Math.max(0, gap),
  );
}

/**
 * Normalizes a wire/reducer plan item status (declared as `string` on the
 * AWAKEN_PLAN_UPDATED frame) into the three-state union the Design System
 * run-plan components accept; unknown values degrade to `pending`.
 */
export function runPlanStepStatusForDisplay(
  status: string,
): "pending" | "in_progress" | "completed" {
  return status === "in_progress" || status === "completed"
    ? status
    : "pending";
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

  const terminalBoundaryIndex = items.findIndex(
    (item) =>
      (item.kind === "answer" && item.isFinal === true) ||
      (item.kind === "hint" && item.isTerminal === true),
  );
  if (terminalBoundaryIndex <= 0) {
    return [
      {
        kind: "items",
        id: `turn-items:${turnId}`,
        items,
      },
    ];
  }

  const executionProcessItems = items.slice(0, terminalBoundaryIndex);
  const answerAndAfterItems = items.slice(terminalBoundaryIndex);
  const elapsedMs = narrativeTurnElapsedMs(items);
  const segments: NarrativeTurnBodySegment[] = [
    {
      kind: "executionProcess",
      id: `execution-process:${turnId}`,
      title: executionProcessSummaryTitle(),
      ...(elapsedMs !== undefined ? { elapsedMs } : {}),
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
  return Math.max(
    0,
    currentScrollTop + promptTop - containerTop - anchorOffset,
  );
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
  return Math.max(
    0,
    containerHeight - anchorOffset - Math.max(0, bottomReserve),
  );
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
  const visibleHeight = Math.max(
    1,
    containerHeight - Math.max(0, bottomReserve),
  );
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
  isAtScrollBottom,
  threshold = 1,
}: {
  previousScrollTop: number | null;
  nextScrollTop: number;
  isAtScrollBottom: boolean;
  threshold?: number;
}): boolean {
  if (previousScrollTop === null || !Number.isFinite(previousScrollTop)) {
    return false;
  }
  // scrollTop 下降但仍贴底是 scrollHeight 收缩时浏览器的被动钳制，
  // 属于布局事件，不是用户上滑意图。
  if (isAtScrollBottom) {
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
