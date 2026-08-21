import type { ReactNode } from "react";
import { AwakenIcon } from "@awaken/design-system";

export type ExpandedFileComposerIdleMode = "rail" | "compact" | "full";
export type ExpandedFileComposerPresentation =
  | ExpandedFileComposerIdleMode
  | "running"
  | "completed";

export type ExpandedFileComposerTrackedRun = {
  threadId: string;
  runId: string;
};

export function hasExpandedFileComposerDraft({
  effectiveText,
  attachmentCount,
  tokenCount,
}: {
  effectiveText: string;
  attachmentCount: number;
  tokenCount: number;
}): boolean {
  return Boolean(
    effectiveText.trim() || attachmentCount > 0 || tokenCount > 0,
  );
}

export function shouldCollapseExpandedFileComposer({
  active,
  busy,
  forceFull,
  hasDraft,
}: {
  active: boolean;
  busy: boolean;
  forceFull: boolean;
  hasDraft: boolean;
}): boolean {
  return active && !busy && !forceFull && !hasDraft;
}

export function expandedFileComposerCompletionKey({
  trackedRun,
  currentThreadId,
  currentRunId,
  status,
  completedAtMs,
}: {
  trackedRun?: ExpandedFileComposerTrackedRun;
  currentThreadId?: string;
  currentRunId?: string;
  status?: string;
  completedAtMs?: number;
}): string | undefined {
  if (
    status !== "finished" ||
    !trackedRun ||
    trackedRun.threadId !== currentThreadId ||
    trackedRun.runId !== currentRunId
  ) {
    return undefined;
  }
  return `${trackedRun.threadId}:${trackedRun.runId}:${
    completedAtMs ?? "finished"
  }`;
}

export function expandedFileComposerPresentation({
  active,
  forceFull,
  running,
  completed,
  idleMode,
}: {
  active: boolean;
  forceFull: boolean;
  running: boolean;
  completed: boolean;
  idleMode: ExpandedFileComposerIdleMode;
}): ExpandedFileComposerPresentation {
  if (!active || forceFull) return "full";
  if (running) return "running";
  if (completed) return "completed";
  return idleMode;
}

export function formatExpandedFileComposerDuration(
  elapsedMs?: number,
): string {
  if (
    typeof elapsedMs !== "number" ||
    !Number.isFinite(elapsedMs) ||
    elapsedMs < 0
  ) {
    return "已完成";
  }

  const totalSeconds = Math.max(1, Math.round(elapsedMs / 1000));
  if (totalSeconds < 60) return `已完成 · ${totalSeconds} 秒`;

  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return seconds > 0
    ? `已完成 · ${minutes} 分 ${seconds} 秒`
    : `已完成 · ${minutes} 分`;
}

export type ExpandedFileComposerProps = {
  active: boolean;
  idleMode: ExpandedFileComposerIdleMode;
  running: boolean;
  completed: boolean;
  completedElapsedMs?: number;
  forceFull?: boolean;
  children: ReactNode;
  onShowCompact: () => void;
  onShowFull: () => void;
  onStop?: () => void;
};

export function ExpandedFileComposer({
  active,
  idleMode,
  running,
  completed,
  completedElapsedMs,
  forceFull = false,
  children,
  onShowCompact,
  onShowFull,
  onStop,
}: ExpandedFileComposerProps) {
  const presentation = expandedFileComposerPresentation({
    active,
    forceFull,
    running,
    completed,
    idleMode,
  });
  const fullVisible = presentation === "full";

  return (
    <div
      className={`expanded-file-composer is-${presentation}`}
      data-expanded-file-composer-state={presentation}
    >
      <div
        className={`expanded-file-composer__full${
          fullVisible ? "" : " is-inactive"
        }`}
        aria-hidden={fullVisible ? undefined : true}
        inert={!fullVisible}
      >
        {children}
      </div>

      {presentation === "rail" ? (
        <button
          type="button"
          className="expanded-file-composer__rail"
          aria-label="展开输入入口"
          onClick={onShowCompact}
        >
          <span aria-hidden="true" />
        </button>
      ) : null}

      {presentation === "compact" ? (
        <button
          type="button"
          className="expanded-file-composer__pill expanded-file-composer__pill-compact"
          aria-label="展开完整输入框"
          onClick={onShowFull}
        >
          <span className="expanded-file-composer__icon" aria-hidden="true">
            <AwakenIcon concept="memory-feedback" size={16} />
          </span>
          <span className="expanded-file-composer__label">继续对话</span>
          <span className="expanded-file-composer__hint">再次点击展开</span>
        </button>
      ) : null}

      {presentation === "running" ? (
        <button
          type="button"
          className="expanded-file-composer__pill expanded-file-composer__pill-running"
          aria-label={onStop ? "正在执行，点击停止任务" : "正在执行"}
          disabled={!onStop}
          onClick={onStop}
        >
          <span
            className="expanded-file-composer__icon expanded-file-composer__spinner"
            aria-hidden="true"
          >
            <AwakenIcon concept="step-running" size={20} />
          </span>
          <span className="expanded-file-composer__label">正在执行</span>
          <span className="expanded-file-composer__hint">点击停止</span>
        </button>
      ) : null}

      {presentation === "completed" ? (
        <button
          type="button"
          className="expanded-file-composer__pill expanded-file-composer__pill-completed"
          aria-label={`${formatExpandedFileComposerDuration(
            completedElapsedMs,
          )}，点击继续对话`}
          onClick={onShowFull}
        >
          <span
            className="expanded-file-composer__completed-dot"
            aria-hidden="true"
          />
          <span className="expanded-file-composer__label">
            {formatExpandedFileComposerDuration(completedElapsedMs)}
          </span>
          <span className="expanded-file-composer__hint">继续对话</span>
        </button>
      ) : null}
    </div>
  );
}
