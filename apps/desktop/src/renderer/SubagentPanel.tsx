import type { ReactNode } from "react";
import {
  AwakenAssistantMessage as AssistantMessage,
  AwakenButton as Button,
  awakenSubagentStatusLabels,
} from "@awaken/design-system";
import type { RunNarrativeItem } from "./run-narrative";
import { projectVisibleNarrativeItems } from "./workbenchPresentation";
import type { SubagentTaskView } from "./runtime/reducer";
import { formatSubagentElapsed, subagentAgentTypeLabel, subagentElapsedMs } from "./subagentPresentation";

export function SubagentPanel({ task, nowMs, cancelling, onCancel, renderActivity }: {
  task: SubagentTaskView;
  nowMs: number;
  cancelling: boolean;
  onCancel: () => void;
  renderActivity: (id: string, items: RunNarrativeItem[], animate: boolean) => ReactNode;
}) {
  const busy = ["queued", "running", "cancelling"].includes(task.status);
  const items = projectVisibleNarrativeItems({
    items: [...task.narrative.committedItems, ...(task.narrative.activeItem ? [task.narrative.activeItem] : [])],
    isBusy: busy,
  });
  const tokens = task.totalTokens ?? task.estimatedTokens;
  const metrics = [
    [task.modelId, task.reasoningEffort].filter(Boolean).join(" · "),
    `${task.toolUseCount} 次工具`,
    tokens === undefined ? undefined : `${tokens.toLocaleString()} tokens`,
    formatSubagentElapsed(subagentElapsedMs(task, nowMs)),
  ].filter(Boolean);
  return <section className="subagent-panel" role="tabpanel"
    id={`subagent-panel-${task.taskId}`} aria-labelledby={`subagent-tab-${task.taskId}`}>
    <header className="subagent-panel__header">
      <span className={`subagent-panel__status subagent-panel__status--${task.status}`}>
        {awakenSubagentStatusLabels[task.status]}
      </span>
      <span className="subagent-panel__role">{subagentAgentTypeLabel(task.agentType)}</span>
      <h2>{task.description}</h2>
      <p className="subagent-panel__metrics">{metrics.join(" · ")}</p>
    </header>
    <div className="subagent-panel__body" data-aw-select-all-root="true">
      {task.contextWindow?.estimatedTokens !== undefined ? <p className="subagent-panel__context">
        当前上下文 {task.contextWindow.estimatedTokens.toLocaleString()}
        {task.contextWindow.effectiveWindow ? ` / ${task.contextWindow.effectiveWindow.toLocaleString()}` : ""} tokens
      </p> : null}
      {task.planItems?.length ? <details className="subagent-panel__plan">
        <summary>任务计划 · {task.planItems.filter(item => item.status === "completed").length}/{task.planItems.length}</summary>
        <ol>{task.planItems.map((item, index) => <li key={index}>
          {item.status === "completed" ? "已完成" : item.status === "in_progress" ? "进行中" : "待处理"} · {item.step}
        </li>)}</ol>
      </details> : null}
      {items.length ? renderActivity(`subagent:${task.taskId}`, items, false)
        : !task.resultPreview && !task.errorMessage ? <p className="subagent-execution-empty">
          {busy ? "等待子任务执行记录…" : "没有执行记录"}
        </p> : null}
      {!items.some(item => item.kind === "answer") && task.resultPreview ? <AssistantMessage
        markdown={task.resultPreview} status={task.status === "failed" ? "error" : "done"} showFooter={false}
      /> : null}
      {task.errorMessage ? <div className="subagent-execution-error" role="alert">
        <strong>{task.errorCode || "子任务失败"}</strong><p>{task.errorMessage}</p>
      </div> : null}
    </div>
    {busy ? <footer className="subagent-panel__footer">
      <Button size="sm" variant="default" onClick={onCancel} disabled={cancelling || task.status === "cancelling"}>
        {cancelling || task.status === "cancelling" ? "正在停止…" : "停止子任务"}
      </Button>
    </footer> : null}
  </section>;
}
