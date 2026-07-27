import "./AwakenScheduledTaskHistoryDialog.css";
import { AwakenButton } from "../AwakenButton";
import { AwakenDialog } from "../AwakenDialog";
import { AwakenScheduledTaskStateBadge } from "../AwakenScheduledTaskStateBadge";
import type { AwakenScheduledTaskExecutionStatus } from "../AwakenScheduledTaskStateBadge";

// 执行记录行：时间与触发方式都是调用方格式化好的字符串；
// 最近 50 条的截断与倒序由调用方负责（契约 §4.4）。
export type AwakenScheduledTaskExecutionItem = {
  executionId: string;
  triggerLabel: string;
  status: AwakenScheduledTaskExecutionStatus;
  statusLabel: string;
  scheduledForText: string;
  startedAtText: string;
  finishedAtText: string;
  errorText?: string | null;
  threadId?: string | null;
};

export type AwakenScheduledTaskHistoryDialogProps = {
  open: boolean;
  title: string;
  items: AwakenScheduledTaskExecutionItem[];
  emptyTitle?: string;
  emptyDescription?: string;
  openThreadLabel?: string;
  scheduledForLabel?: string;
  startedAtLabel?: string;
  finishedAtLabel?: string;
  onOpenChange: (open: boolean) => void;
  onOpenThread?: (execution: AwakenScheduledTaskExecutionItem) => void;
};

export function AwakenScheduledTaskHistoryDialog({
  open,
  title,
  items,
  emptyTitle = "还没有执行记录",
  emptyDescription = "任务按计划或手动触发后，最近的执行结果会显示在这里。",
  openThreadLabel = "打开线程",
  scheduledForLabel = "计划",
  startedAtLabel = "开始",
  finishedAtLabel = "结束",
  onOpenChange,
  onOpenThread,
}: AwakenScheduledTaskHistoryDialogProps) {
  return (
    <AwakenDialog
      open={open}
      className="aw-scheduled-task-history-dialog"
      title={title}
      onOpenChange={onOpenChange}
    >
      {items.length === 0 ? (
        <div className="aw-scheduled-task-history__empty">
          <strong className="aw-scheduled-task-history__empty-title">{emptyTitle}</strong>
          <p className="aw-scheduled-task-history__empty-description">{emptyDescription}</p>
        </div>
      ) : (
        <ul className="aw-scheduled-task-history__list" aria-label={title}>
          {items.map((item) => (
            <li className="aw-scheduled-task-history__row" key={item.executionId} data-status={item.status}>
              <span className="aw-scheduled-task-history__row-copy">
                <span className="aw-scheduled-task-history__row-title-line">
                  <AwakenScheduledTaskStateBadge state={item.status} label={item.statusLabel} />
                  <span className="aw-scheduled-task-history__row-trigger">{item.triggerLabel}</span>
                </span>
                <span className="aw-scheduled-task-history__row-times">
                  <span>{scheduledForLabel} {item.scheduledForText}</span>
                  <span>{startedAtLabel} {item.startedAtText}</span>
                  <span>{finishedAtLabel} {item.finishedAtText}</span>
                </span>
                {item.errorText ? (
                  <span className="aw-scheduled-task-history__row-error">{item.errorText}</span>
                ) : null}
              </span>
              {item.threadId && onOpenThread ? (
                <span className="aw-scheduled-task-history__row-actions">
                  <AwakenButton variant="default" size="sm" onClick={() => onOpenThread(item)}>
                    {openThreadLabel}
                  </AwakenButton>
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </AwakenDialog>
  );
}
