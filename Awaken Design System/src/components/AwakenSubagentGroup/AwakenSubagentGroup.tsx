import "./AwakenSubagentGroup.css";
import { awakenSubagentStatusLabels as statusLabels } from "./subagentStatus";

export type AwakenSubagentRowView = {
  taskId: string;
  description: string;
  agentType: "explore" | "plan" | "review" | "general";
  status: "queued" | "running" | "completed" | "failed" | "cancelling" | "cancelled" | "interrupted";
  activity?: string;
  resultPreview?: string;
  toolUseCount: number;
  estimatedTokens?: number;
  totalTokens?: number;
  elapsedMs?: number;
  selected?: boolean;
};

export type AwakenSubagentGroupProps = {
  tasks: AwakenSubagentRowView[];
  onSelect?: (taskId: string) => void;
  onStopAll?: () => void;
};

const groupStatusLabels = {
  ...statusLabels,
  running: "进行中",
  completed: "完成",
};

const busyStatuses = new Set(["queued", "running", "cancelling"]);

export function AwakenSubagentGroup({
  tasks,
  onSelect,
  onStopAll,
}: AwakenSubagentGroupProps) {
  const canStopAll =
    Boolean(onStopAll) && tasks.some((task) => busyStatuses.has(task.status));
  return (
    <section className="aw-subagent-group" aria-label="subagent">
      <header className="aw-subagent-group__header">
        <strong>subagent</strong>
        {canStopAll ? (
          <button
            type="button"
            className="aw-subagent-group__stop-all"
            onClick={() => onStopAll?.()}
          >
            停止全部任务
          </button>
        ) : null}
      </header>
      <div className="aw-subagent-group__rows">
        {tasks.map((task) => (
          <button
            key={task.taskId}
            type="button"
            className={`aw-subagent-row aw-subagent-row--${task.status}${task.selected ? " is-selected" : ""}`}
            aria-label={`${task.description}，${groupStatusLabels[task.status]}`}
            onClick={() => onSelect?.(task.taskId)}
          >
            <strong className="aw-subagent-row__name">{task.description}</strong>
            <span className="aw-subagent-row__label">{groupStatusLabels[task.status]}</span>
          </button>
        ))}
      </div>
    </section>
  );
}
