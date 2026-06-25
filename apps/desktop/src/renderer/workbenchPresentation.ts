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
}: {
  containerHeight: number;
  anchorOffset: number;
}): number {
  return Math.max(0, containerHeight - anchorOffset);
}
