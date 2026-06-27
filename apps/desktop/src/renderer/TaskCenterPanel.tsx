import { RefreshCcw, Rows3 } from "lucide-react";
import type { ThreadItem } from "./runtime/reducer";
import { describeThreadStatus, formatThreadUpdatedAt, splitTaskCenterThreads } from "./threadPresentation";

type TaskCenterPanelProps = {
  open: boolean;
  activeThreadId?: string;
  errorMessage?: string | null;
  threads: ThreadItem[];
  onRefresh: () => void;
  onSelectThread: (threadId: string) => void;
  onToggle: () => void;
};

function renderThreadRow(
  thread: ThreadItem,
  activeThreadId: string | undefined,
  onSelectThread: (threadId: string) => void,
) {
  const status = describeThreadStatus(thread);
  return (
    <button
      key={thread.threadId}
      type="button"
      className={`task-center-row${thread.threadId === activeThreadId ? " task-center-row-active" : ""}`}
      onClick={() => onSelectThread(thread.threadId)}
    >
      <span className="task-center-row-head">
        <strong>{thread.label}</strong>
        {thread.threadId === activeThreadId ? (
          <span className="task-center-current-pill">Current</span>
        ) : null}
      </span>
      <span className="task-center-row-meta">
        <span className={`task-center-status-pill task-center-status-${status.tone}`}>
          {status.label}
        </span>
        <span>Updated {formatThreadUpdatedAt(thread.updatedAt)}</span>
      </span>
      {thread.lastError ? (
        <span className="task-center-row-error">{thread.lastError}</span>
      ) : null}
    </button>
  );
}

export function TaskCenterPanel({
  open,
  activeThreadId,
  errorMessage,
  threads,
  onRefresh,
  onSelectThread,
  onToggle,
}: TaskCenterPanelProps) {
  const { running, recent } = splitTaskCenterThreads(threads);
  const runningCount = running.length;

  return (
    <div className="task-center-anchor">
      <button
        type="button"
        className={`task-center-toggle${open ? " task-center-toggle-open" : ""}`}
        aria-expanded={open}
        aria-haspopup="dialog"
        aria-label="Toggle task center"
        onClick={onToggle}
      >
        <Rows3 size={14} strokeWidth={1.9} />
        <span>Tasks</span>
        <span className="task-center-toggle-count">{runningCount}</span>
      </button>

      {open ? (
        <section className="task-center-panel" aria-label="Task center">
          <div className="task-center-panel-head">
            <div>
              <strong>Task center</strong>
              <span>{runningCount > 0 ? `${runningCount} running` : "Recent runs"}</span>
            </div>
            <button
              type="button"
              className="task-center-refresh"
              aria-label="Refresh task center"
              onClick={onRefresh}
            >
              <RefreshCcw size={13} strokeWidth={1.85} />
            </button>
          </div>

          {errorMessage ? (
            <div className="task-center-empty task-center-empty-error">
              <strong>Task list unavailable</strong>
              <p>{errorMessage}</p>
            </div>
          ) : threads.length === 0 ? (
            <div className="task-center-empty">
              <strong>No runs yet</strong>
              <p>New agent runs will appear here after the first thread starts.</p>
            </div>
          ) : (
            <div className="task-center-sections">
              {running.length > 0 ? (
                <section className="task-center-section">
                  <div className="task-center-section-head">
                    <span>Running</span>
                    <span>{running.length}</span>
                  </div>
                  <div className="task-center-list">
                    {running.map((thread) => renderThreadRow(thread, activeThreadId, onSelectThread))}
                  </div>
                </section>
              ) : null}

              <section className="task-center-section">
                <div className="task-center-section-head">
                  <span>Recent</span>
                  <span>{recent.length}</span>
                </div>
                {recent.length > 0 ? (
                  <div className="task-center-list">
                    {recent.slice(0, 8).map((thread) =>
                      renderThreadRow(thread, activeThreadId, onSelectThread)
                    )}
                  </div>
                ) : (
                  <div className="task-center-empty task-center-empty-inline">
                    <p>No completed or failed runs yet.</p>
                  </div>
                )}
              </section>
            </div>
          )}
        </section>
      ) : null}
    </div>
  );
}
