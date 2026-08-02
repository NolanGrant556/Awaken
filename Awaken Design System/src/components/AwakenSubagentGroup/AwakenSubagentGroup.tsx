import "./AwakenSubagentGroup.css";

export type AwakenSubagentRowView = {
  taskId: string;
  description: string;
  agentType: "explore" | "review" | "general";
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
};

const statusLabels: Record<AwakenSubagentRowView["status"], string> = {
  queued: "排队中",
  running: "运行中",
  completed: "已完成",
  failed: "失败",
  cancelling: "正在取消",
  cancelled: "已取消",
  interrupted: "已中断",
};

function formatMetrics(task: AwakenSubagentRowView): string {
  const metrics: string[] = [];
  if (task.toolUseCount > 0) metrics.push(`${task.toolUseCount} 次工具`);
  const tokens = task.totalTokens ?? task.estimatedTokens;
  if (typeof tokens === "number" && tokens > 0) {
    metrics.push(`${new Intl.NumberFormat("zh-CN").format(tokens)} tokens`);
  }
  if (typeof task.elapsedMs === "number" && Number.isFinite(task.elapsedMs) && task.elapsedMs >= 0) {
    const seconds = Math.max(1, Math.round(task.elapsedMs / 1000));
    metrics.push(seconds < 60 ? `${seconds} 秒` : `${Math.floor(seconds / 60)} 分 ${seconds % 60} 秒`);
  }
  return metrics.join(" · ");
}

export function AwakenSubagentGroup({ tasks, onSelect }: AwakenSubagentGroupProps) {
  const running = tasks.filter((task) =>
    ["queued", "running", "cancelling"].includes(task.status),
  ).length;
  const completed = tasks.filter((task) => task.status === "completed").length;
  return (
    <section className="aw-subagent-group" aria-label="子任务">
      <header className="aw-subagent-group__header">
        <strong>{tasks.length} 个子任务</strong>
        <span>
          {running ? `${running} 运行中` : ""}
          {running && completed ? " · " : ""}
          {completed ? `${completed} 已完成` : ""}
        </span>
      </header>
      <div className="aw-subagent-group__rows">
        {tasks.map((task) => {
          const metrics = formatMetrics(task);
          return (
            <button
              key={task.taskId}
              type="button"
              className={`aw-subagent-row aw-subagent-row--${task.status}${task.selected ? " is-selected" : ""}`}
              aria-label={`${task.description}，${statusLabels[task.status]}`}
              onClick={() => onSelect?.(task.taskId)}
            >
              <span className="aw-subagent-row__status" aria-hidden="true" />
              <span className="aw-subagent-row__body">
                <span className="aw-subagent-row__title">
                  <strong>{task.description}</strong>
                  <small>{task.agentType}</small>
                </span>
                <span className="aw-subagent-row__detail">
                  {task.resultPreview || task.activity || statusLabels[task.status]}
                </span>
                {metrics ? <span className="aw-subagent-row__metrics">{metrics}</span> : null}
              </span>
              <span className="aw-subagent-row__label">{statusLabels[task.status]}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
