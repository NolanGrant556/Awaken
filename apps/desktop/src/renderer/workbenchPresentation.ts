import type { RunNarrativeItem } from "./run-narrative";

export type ResolvedApprovalState = {
  approvalId: string;
  decision: "approved" | "rejected";
} | null;

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

export function resolvedDecisionForApproval(
  activeApproval: { id: string } | undefined,
  resolvedApproval: ResolvedApprovalState,
): "approved" | "rejected" | undefined {
  if (!activeApproval || !resolvedApproval || resolvedApproval.approvalId !== activeApproval.id) {
    return undefined;
  }
  return resolvedApproval.decision;
}

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
  hasNotice,
  hasError,
  needsRestart,
}: {
  hasNarrative: boolean;
  hasRun: boolean;
  hasNotice: boolean;
  hasError: boolean;
  needsRestart: boolean;
}): boolean {
  return !hasNarrative && !hasRun && !hasNotice && !hasError && !needsRestart;
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

export function composerBottomSafeArea({
  composerHeight,
  safeGap,
}: {
  composerHeight: number;
  safeGap: number;
}): number {
  return Math.max(0, composerHeight) + Math.max(0, safeGap);
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

export function scrollTopForLatestTurnOutputTail({
  currentScrollTop,
  containerTop,
  outputBottom,
  containerHeight,
  bottomReserve,
  anchorScrollTop,
}: {
  currentScrollTop: number;
  containerTop: number;
  outputBottom: number;
  containerHeight: number;
  bottomReserve: number;
  anchorScrollTop: number;
}): number {
  const visibleHeight = Math.max(1, containerHeight - Math.max(0, bottomReserve));
  const scrollTopForTail =
    currentScrollTop + outputBottom - containerTop - visibleHeight;
  return Math.max(0, anchorScrollTop, scrollTopForTail);
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

export function shouldPauseLatestOutputFollowForWheel({
  deltaY,
}: {
  deltaY: number;
}): boolean {
  return deltaY < -1;
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
  hasResumeIntent,
}: {
  isPausedByUser: boolean;
  isNearTail: boolean;
  hasResumeIntent: boolean;
}): boolean {
  if (!isNearTail) {
    return false;
  }
  if (!isPausedByUser) {
    return true;
  }
  return hasResumeIntent;
}
