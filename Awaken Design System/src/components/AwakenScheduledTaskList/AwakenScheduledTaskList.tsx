import "./AwakenScheduledTaskList.css";
import { useState } from "react";
import { CalendarClock, MoreHorizontal } from "../../icons";
import { AwakenButton } from "../AwakenButton";
import { AwakenDropdown } from "../AwakenDropdown";
import { AwakenSwitch } from "../AwakenSwitch";
import { AwakenScheduledTaskStateBadge } from "../AwakenScheduledTaskStateBadge";
import type { AwakenScheduledTaskState } from "../AwakenScheduledTaskStateBadge";

// 行数据全部是人类可读字符串，由调用方格式化（类型标签、计划+时区、下次/最近执行）；
// 组件不解析 cron、不做时区换算，也不感知 Core payload。
export type AwakenScheduledTaskListItem = {
  taskId: string;
  name: string;
  kindLabel: string;
  projectName: string;
  scheduleText: string;
  enabled: boolean;
  state: AwakenScheduledTaskState;
  stateLabel: string;
  nextRunText?: string | null;
  lastRunText?: string | null;
  running: boolean;
};

export type AwakenScheduledTaskListProps = {
  items: AwakenScheduledTaskListItem[];
  loading?: boolean;
  listAriaLabel?: string;
  loadingLabel?: string;
  emptyTitle?: string;
  emptyDescription?: string;
  createLabel?: string;
  createFromTemplateLabel?: string;
  runNowLabel?: string;
  cancelRunLabel?: string;
  historyLabel?: string;
  editLabel?: string;
  deleteLabel?: string;
  moreActionsLabel?: string;
  onCreate?: () => void;
  onCreateFromTemplate?: () => void;
  onToggleEnabled?: (taskId: string, enabled: boolean) => void;
  onRunNow?: (taskId: string) => void;
  onCancelRun?: (taskId: string) => void;
  onShowHistory?: (taskId: string) => void;
  onEdit?: (taskId: string) => void;
  onDelete?: (taskId: string) => void;
};

type RowLabels = {
  runNowLabel: string;
  cancelRunLabel: string;
  historyLabel: string;
  editLabel: string;
  deleteLabel: string;
  moreActionsLabel: string;
};

type RowCallbacks = {
  onToggleEnabled?: AwakenScheduledTaskListProps["onToggleEnabled"];
  onRunNow?: AwakenScheduledTaskListProps["onRunNow"];
  onCancelRun?: AwakenScheduledTaskListProps["onCancelRun"];
  onShowHistory?: AwakenScheduledTaskListProps["onShowHistory"];
  onEdit?: AwakenScheduledTaskListProps["onEdit"];
  onDelete?: AwakenScheduledTaskListProps["onDelete"];
};

type RowMenuAction = {
  id: string;
  label: string;
  tone?: "danger";
  disabled?: boolean;
  onSelect?: (taskId: string) => void;
};

function ScheduledTaskRow({
  item,
  labels,
  callbacks,
}: {
  item: AwakenScheduledTaskListItem;
  labels: RowLabels;
  callbacks: RowCallbacks;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const runMenuItem = item.running
    ? { id: "cancel-run", label: labels.cancelRunLabel, onSelect: callbacks.onCancelRun }
    : { id: "run-now", label: labels.runNowLabel, onSelect: callbacks.onRunNow };
  // 契约 §4.4：运行中的任务禁止删除，先取消或等待结束。
  const menuItems: RowMenuAction[] = [
    runMenuItem,
    { id: "history", label: labels.historyLabel, onSelect: callbacks.onShowHistory },
    { id: "edit", label: labels.editLabel, onSelect: callbacks.onEdit },
    { id: "delete", label: labels.deleteLabel, onSelect: callbacks.onDelete, tone: "danger" as const, disabled: item.running },
  ];

  return (
    <li className="aw-scheduled-task-list__row" data-state={item.state} data-running={item.running ? "true" : "false"}>
      <span className="aw-scheduled-task-list__row-icon" aria-hidden="true">
        <CalendarClock size={16} strokeWidth={1.9} />
      </span>
      <span className="aw-scheduled-task-list__row-copy">
        <span className="aw-scheduled-task-list__row-title-line">
          <strong className="aw-scheduled-task-list__row-title">{item.name}</strong>
          <AwakenScheduledTaskStateBadge state={item.state} label={item.stateLabel} />
          <span className="aw-scheduled-task-list__row-kind">{item.kindLabel}</span>
        </span>
        <span className="aw-scheduled-task-list__row-meta">{item.projectName} · {item.scheduleText}</span>
        {item.nextRunText || item.lastRunText ? (
          <span className="aw-scheduled-task-list__row-meta">
            {item.nextRunText ?? ""}
            {item.nextRunText && item.lastRunText ? " · " : ""}
            {item.lastRunText ?? ""}
          </span>
        ) : null}
      </span>
      <span className="aw-scheduled-task-list__row-actions">
        <AwakenSwitch
          checked={item.enabled}
          label={item.name}
          onCheckedChange={(enabled) => callbacks.onToggleEnabled?.(item.taskId, enabled)}
        />
        <AwakenDropdown
          open={menuOpen}
          onOpenChange={setMenuOpen}
          placement="bottom-end"
          offset={4}
          trigger={
            <button
              type="button"
              className="aw-scheduled-task-list__row-menu-trigger"
              aria-label={`${item.name} ${labels.moreActionsLabel}`}
            >
              <MoreHorizontal size={16} strokeWidth={1.9} />
            </button>
          }
        >
          <div className="aw-scheduled-task-list__menu" role="menu">
            {menuItems.map((action) => (
              <button
                key={action.id}
                type="button"
                role="menuitem"
                className={[
                  "aw-scheduled-task-list__menu-item",
                  action.tone === "danger" ? "aw-scheduled-task-list__menu-item--danger" : "",
                ].filter(Boolean).join(" ")}
                disabled={action.disabled}
                onClick={() => {
                  setMenuOpen(false);
                  action.onSelect?.(item.taskId);
                }}
              >
                {action.label}
              </button>
            ))}
          </div>
        </AwakenDropdown>
      </span>
    </li>
  );
}

export function AwakenScheduledTaskList({
  items,
  loading = false,
  listAriaLabel = "定时任务列表",
  loadingLabel = "正在加载定时任务…",
  emptyTitle = "还没有定时任务",
  emptyDescription = "创建固定提醒或 Agent 任务，按计划自动执行；Awaken 退出期间不会运行。",
  createLabel = "创建任务",
  createFromTemplateLabel = "从模板创建",
  runNowLabel = "立即执行",
  cancelRunLabel = "取消执行",
  historyLabel = "执行记录",
  editLabel = "编辑",
  deleteLabel = "删除",
  moreActionsLabel = "更多操作",
  onCreate,
  onCreateFromTemplate,
  onToggleEnabled,
  onRunNow,
  onCancelRun,
  onShowHistory,
  onEdit,
  onDelete,
}: AwakenScheduledTaskListProps) {
  if (loading) {
    return (
      <div className="aw-scheduled-task-list__loading" role="status">
        {loadingLabel}
      </div>
    );
  }

  if (items.length === 0) {
    // 契约 §4.1：空状态必须同时提供普通创建与从模板创建，不展示静态假任务。
    return (
      <div className="aw-scheduled-task-list__empty">
        <strong className="aw-scheduled-task-list__empty-title">{emptyTitle}</strong>
        <p className="aw-scheduled-task-list__empty-description">{emptyDescription}</p>
        <div className="aw-scheduled-task-list__empty-actions">
          <AwakenButton variant="primary" size="md" onClick={onCreate}>{createLabel}</AwakenButton>
          <AwakenButton variant="default" size="md" onClick={onCreateFromTemplate}>{createFromTemplateLabel}</AwakenButton>
        </div>
      </div>
    );
  }

  const labels: RowLabels = { runNowLabel, cancelRunLabel, historyLabel, editLabel, deleteLabel, moreActionsLabel };
  const callbacks: RowCallbacks = { onToggleEnabled, onRunNow, onCancelRun, onShowHistory, onEdit, onDelete };

  return (
    <ul className="aw-scheduled-task-list" aria-label={listAriaLabel}>
      {items.map((item) => (
        <ScheduledTaskRow key={item.taskId} item={item} labels={labels} callbacks={callbacks} />
      ))}
    </ul>
  );
}
