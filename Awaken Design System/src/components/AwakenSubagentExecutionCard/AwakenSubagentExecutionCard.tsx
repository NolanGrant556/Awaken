import type { ReactNode } from "react";
import "./AwakenSubagentExecutionCard.css";

export type AwakenSubagentExecutionStatus =
  | "queued"
  | "running"
  | "completed"
  | "failed"
  | "cancelling"
  | "cancelled"
  | "interrupted";

export const awakenSubagentStatusLabels: Record<
  AwakenSubagentExecutionStatus,
  string
> = {
  queued: "排队中",
  running: "运行中",
  completed: "已完成",
  failed: "失败",
  cancelling: "正在取消",
  cancelled: "已取消",
  interrupted: "已中断",
};

export type AwakenSubagentExecutionCardProps = {
  title: string;
  agentLabel: string;
  status: AwakenSubagentExecutionStatus;
  activity?: string;
  metrics?: string[];
  children?: ReactNode;
  onClose: () => void;
  onCancel?: () => void;
  cancelDisabled?: boolean;
};

export function AwakenSubagentExecutionCard({
  title,
  agentLabel,
  status,
  activity,
  metrics = [],
  children,
  onClose,
  onCancel,
  cancelDisabled = false,
}: AwakenSubagentExecutionCardProps) {
  const headerMeta = [...metrics, awakenSubagentStatusLabels[status]].join(" · ");

  return (
    <section
      className={`aw-subagent-execution-card aw-subagent-execution-card--${status}`}
      aria-label={`子任务详情：${title}`}
    >
      <header className="aw-subagent-execution-card__header">
        <span className="aw-subagent-execution-card__status-dot" aria-hidden="true" />
        <span className="aw-subagent-execution-card__heading">
          <span className="aw-subagent-execution-card__agent-label">{agentLabel}：</span>
          <strong>{title}</strong>
        </span>
        <small className="aw-subagent-execution-card__metrics">{headerMeta}</small>
        <button
          type="button"
          className="aw-subagent-execution-card__close"
          aria-label="关闭子任务详情"
          onClick={onClose}
        >
          ×
        </button>
      </header>
      {activity ? (
        <div className="aw-subagent-execution-card__summary">
          <span>{activity}</span>
        </div>
      ) : null}
      <div className="aw-subagent-execution-card__body">{children}</div>
      {onCancel ? (
        <footer className="aw-subagent-execution-card__footer">
          <button
            type="button"
            className="aw-subagent-execution-card__cancel"
            disabled={cancelDisabled}
            onClick={onCancel}
          >
            {cancelDisabled ? "正在取消…" : "停止子任务"}
          </button>
        </footer>
      ) : null}
    </section>
  );
}
