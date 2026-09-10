import type { SubagentNarrativeTask } from "./run-narrative";

export function subagentElapsedMs(
  task: Pick<SubagentNarrativeTask, "startedAt" | "finishedAt">,
  nowMs: number,
): number | undefined {
  const startedAt = task.startedAt ? Date.parse(task.startedAt) : Number.NaN;
  const finishedAt = task.finishedAt ? Date.parse(task.finishedAt) : nowMs;
  if (
    !Number.isFinite(startedAt) ||
    !Number.isFinite(finishedAt) ||
    finishedAt < startedAt
  ) {
    return undefined;
  }
  return finishedAt - startedAt;
}

export function subagentWorkspaceModeLabel(mode?: string): string | undefined {
  switch (mode) {
    case "shared":
      return "共享可写";
    case "git_worktree":
      return "Git worktree";
    case "shared_readonly":
      return "只读共享";
    default:
      return mode || undefined;
  }
}

export function formatSubagentElapsed(elapsedMs?: number): string | undefined {
  if (
    typeof elapsedMs !== "number" ||
    !Number.isFinite(elapsedMs) ||
    elapsedMs < 0
  ) {
    return undefined;
  }
  const totalSeconds = Math.max(1, Math.round(elapsedMs / 1000));
  if (totalSeconds < 60) return `${totalSeconds} 秒`;
  return `${Math.floor(totalSeconds / 60)} 分 ${totalSeconds % 60} 秒`;
}

export function subagentAgentTypeLabel(
  agentType: SubagentNarrativeTask["agentType"],
): string {
  switch (agentType) {
    case "review":
      return "复核智能体";
    case "plan":
      return "规划智能体";
    case "general":
      return "通用智能体";
    default:
      return "探索智能体";
  }
}
