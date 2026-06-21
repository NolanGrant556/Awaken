export type ResolvedApprovalState = {
  approvalId: string;
  decision: "approved" | "rejected";
} | null;

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
