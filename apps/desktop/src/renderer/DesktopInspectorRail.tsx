import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";
import {
  AwakenAssistantMessage as AssistantMessage,
  AwakenIcon,
  AwakenButton as Button,
  AwakenInspectorRail as InspectorRail,
  awakenSubagentStatusLabels,
} from "@awaken/design-system";
import { INSPECTOR_FILE_ITEMS } from "./desktopLayoutConstants";
import { type SubagentTaskView } from "./runtime/reducer";
import {
  formatSubagentElapsed,
  subagentAgentTypeLabel,
  subagentElapsedMs,
  subagentWorkspaceModeLabel,
} from "./subagentPresentation";
import { useWorkbenchPanels } from "./useWorkbenchPanels";
type Props = {
  workbenchPanels: Pick<
    ReturnType<typeof useWorkbenchPanels>,
    "selectedSubagentTaskId" | "setSelectedSubagentTaskId"
  >;
  subagentTasks: SubagentTaskView[];
  runningSubagentCount: number;
  handleOpenFileWorkspace: (mode?: ProjectFileWorkspaceMode) => void;
  handleToggleSubagentInspector: () => void;
  selectedSubagent: SubagentTaskView | undefined;
  subagentNowMs: number;
  cancellingSubagentTaskId: string | undefined;
  handleCancelSubagent: (taskId: string) => Promise<void>;
};
export function DesktopInspectorRail({
  workbenchPanels,
  subagentTasks,
  runningSubagentCount,
  handleOpenFileWorkspace,
  handleToggleSubagentInspector,
  selectedSubagent,
  subagentNowMs,
  cancellingSubagentTaskId,
  handleCancelSubagent,
}: Props) {
  const { selectedSubagentTaskId, setSelectedSubagentTaskId } = workbenchPanels;
  return (
    <InspectorRail
      ariaLabel={selectedSubagentTaskId ? "子任务" : "文件"}
      activeId={selectedSubagentTaskId ? "subagents" : undefined}
      items={[
        ...INSPECTOR_FILE_ITEMS,
        ...(subagentTasks.length
          ? [
              {
                id: "subagents",
                label: "子任务",
                badge: runningSubagentCount || subagentTasks.length,
                tone: runningSubagentCount
                  ? ("warning" as const)
                  : ("neutral" as const),
                icon: <AwakenIcon concept="subagent" size={16} />,
              },
            ]
          : []),
      ]}
      onSelect={(id) => {
        if (id === "files") {
          handleOpenFileWorkspace("files");
        } else if (id === "browser") {
          handleOpenFileWorkspace("browser");
        } else if (id === "subagents") {
          handleToggleSubagentInspector();
        }
      }}
    >
      {selectedSubagentTaskId && selectedSubagent ? (
        <div className="subagent-inspector">
          <header className="subagent-inspector__header">
            <div>
              <span className="subagent-inspector__eyebrow">
                {subagentAgentTypeLabel(selectedSubagent.agentType)}
                {subagentWorkspaceModeLabel(selectedSubagent.workspaceMode)
                  ? ` · ${subagentWorkspaceModeLabel(selectedSubagent.workspaceMode)}`
                  : ""}
              </span>
              <h3>{selectedSubagent.description}</h3>
            </div>
            <span
              className={`subagent-inspector__status subagent-inspector__status--${selectedSubagent.status}`}
            >
              {awakenSubagentStatusLabels[selectedSubagent.status]}
            </span>
          </header>
          <div className="subagent-inspector__metrics" aria-label="子任务统计">
            <span>{selectedSubagent.toolUseCount} 次工具</span>
            {(selectedSubagent.totalTokens ??
            selectedSubagent.estimatedTokens) ? (
              <span>
                {new Intl.NumberFormat("zh-CN").format(
                  selectedSubagent.totalTokens ??
                    selectedSubagent.estimatedTokens ??
                    0,
                )}{" "}
                tokens
              </span>
            ) : null}
            {formatSubagentElapsed(
              subagentElapsedMs(selectedSubagent, subagentNowMs),
            ) ? (
              <span>
                {formatSubagentElapsed(
                  subagentElapsedMs(selectedSubagent, subagentNowMs),
                )}
              </span>
            ) : null}
          </div>
          <div className="subagent-inspector__switcher">
            {subagentTasks.map((task) => (
              <button
                key={task.taskId}
                type="button"
                className={
                  task.taskId === selectedSubagent.taskId
                    ? "is-active"
                    : undefined
                }
                onClick={() => setSelectedSubagentTaskId(task.taskId)}
              >
                {task.description}
              </button>
            ))}
          </div>
          <section>
            <h4>最近活动</h4>
            <p>
              {selectedSubagent.activity ||
                awakenSubagentStatusLabels[selectedSubagent.status]}
            </p>
          </section>
          {selectedSubagent.resultPreview ? (
            <section>
              <h4>结果</h4>
              <AssistantMessage
                markdown={selectedSubagent.resultPreview}
                status={selectedSubagent.status === "failed" ? "error" : "done"}
                showFooter={false}
              />
            </section>
          ) : null}
          {selectedSubagent.errorCode ? (
            <section className="subagent-inspector__error">
              <h4>错误</h4>
              <p>{selectedSubagent.errorCode}</p>
              {selectedSubagent.errorMessage ? (
                <p>{selectedSubagent.errorMessage}</p>
              ) : null}
            </section>
          ) : null}
          {["queued", "running"].includes(selectedSubagent.status) ? (
            <Button
              variant="default"
              size="sm"
              disabled={cancellingSubagentTaskId === selectedSubagent.taskId}
              onClick={() => void handleCancelSubagent(selectedSubagent.taskId)}
            >
              {cancellingSubagentTaskId === selectedSubagent.taskId
                ? "正在取消…"
                : "取消子任务"}
            </Button>
          ) : null}
        </div>
      ) : null}
    </InspectorRail>
  );
}
