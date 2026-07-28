import "./AwakenSubagentGroup.css";

export type AwakenSubagentRowView = {
  taskId: string;
  description: string;
  agentType: "explore" | "review" | "general";
  status: "queued" | "running" | "completed" | "failed" | "cancelling" | "cancelled" | "interrupted";
  activity?: string;
  resultPreview?: string;
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
        {tasks.map((task) => (
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
            </span>
            <span className="aw-subagent-row__label">{statusLabels[task.status]}</span>
          </button>
        ))}
      </div>
    </section>
  );
}
