import { useEffect, useRef } from "react";
import {
  AwakenIcon,
  AwakenTooltip as Tooltip,
  AwakenWorkbenchChrome as WorkbenchChrome,
  type PlatformKind,
} from "@awaken/design-system";
import ProjectFileIcon from "./ProjectFileIcon";
import { artifactPreviewTabMediaIconConcept } from "./artifactMediaPresentation";
import { projectFileIconKind } from "./projectFiles";
import type { useFileWorkspaceSession } from "./useFileWorkspaceSession";
import type { SubagentTaskView } from "./runtime/reducer";

type Props = {
  fileWorkspace: Pick<
    ReturnType<typeof useFileWorkspaceSession>,
    | "expandedWorkspaceSurface"
    | "fileWorkspaceExpandedActive"
    | "fileWorkspaceOpen"
    | "fileWorkspacePanelAction"
    | "fileWorkspaceTabsAreActive"
    | "handleActivateInspectorTab"
    | "handleAddProjectFileTab"
    | "handleCloseFileWorkspace"
    | "handleContractFileWorkspace"
    | "handleExpandFileWorkspace"
    | "handleOpenFileWorkspace"
    | "handleShowConversationSurface"
    | "handleToggleInspectorCollapsed"
    | "inspectorChooserActive"
    | "inspectorCollapsed"
    | "inspectorPresentation"
    | "inspectorTabs"
    | "setSidebarCollapsed"
    | "showFileWorkspacePlaceholder"
    | "sidebarCollapsed"
  >;
  platform: PlatformKind;
  handleNewTask: () => void;
  currentConversationLabel: string;
  subagentTabs: SubagentTaskView[];
  onOpenSubagent: (taskId: string) => void;
};
export function DesktopWorkbenchChrome({
  fileWorkspace: files,
  platform,
  handleNewTask,
  currentConversationLabel,
  subagentTabs,
  onOpenSubagent,
}: Props) {
  const tabsRef = useRef<HTMLDivElement>(null);
  const { state, dispatch } = files.inspectorTabs;
  useEffect(() => {
    tabsRef.current
      ?.querySelector('[role="tab"][aria-selected="true"]')
      ?.scrollIntoView?.({ block: "nearest", inline: "nearest" });
  }, [state.activeId]);
  const panelToggle = (
    <button
      type="button"
      className="workbench-chrome-icon workbench-chrome-panel-toggle"
      aria-label={
        files.inspectorCollapsed ? "Expand inspector" : "Collapse inspector"
      }
      onClick={files.handleToggleInspectorCollapsed}
    >
      <AwakenIcon concept="toggle-right-panel" size={14} />
    </button>
  );
  const expanded = files.fileWorkspaceExpandedActive;
  return (
    <WorkbenchChrome
      platform={platform}
      sidebarCollapsed={files.sidebarCollapsed}
      onToggleSidebar={() => files.setSidebarCollapsed((value) => !value)}
      onNewTask={handleNewTask}
      center={
        expanded && files.inspectorPresentation.available ? (
          <div
            className="workbench-conversation-tabs"
            role="tablist"
            aria-label="当前会话"
          >
            <button
              type="button"
              className={`workbench-conversation-tab${files.expandedWorkspaceSurface === "conversation" ? " is-active" : ""}`}
              role="tab"
              aria-selected={files.expandedWorkspaceSurface === "conversation"}
              title={currentConversationLabel}
              onClick={files.handleShowConversationSurface}
            >
              <AwakenIcon concept="memory-feedback" size={16} />
              <span>{currentConversationLabel}</span>
            </button>
          </div>
        ) : undefined
      }
      right={
        files.inspectorPresentation.available &&
        (files.fileWorkspaceOpen ||
          state.tabs.length > 0 ||
          files.inspectorChooserActive) ? (
          <div className="workbench-chrome-file-mode">
            <div className="project-files-tab-rail">
              <div
                ref={tabsRef}
                className="project-files-tabs"
                role="tablist"
                aria-label="打开的面板"
                onKeyDown={(event) => {
                  if (
                    !["ArrowLeft", "ArrowRight", "Home", "End"].includes(
                      event.key,
                    ) ||
                    !(event.target instanceof HTMLElement) ||
                    event.target.getAttribute("role") !== "tab"
                  )
                    return;
                  const tabs = Array.from(
                    event.currentTarget.querySelectorAll<HTMLButtonElement>(
                      '[role="tab"]',
                    ),
                  );
                  const current = tabs.indexOf(
                    event.target as HTMLButtonElement,
                  );
                  const next =
                    event.key === "Home"
                      ? 0
                      : event.key === "End"
                        ? tabs.length - 1
                        : (current +
                            (event.key === "ArrowRight" ? 1 : -1) +
                            tabs.length) %
                          tabs.length;
                  event.preventDefault();
                  tabs[next]?.focus({ preventScroll: true });
                  tabs[next]?.click();
                  tabs[next]?.scrollIntoView?.({
                    block: "nearest",
                    inline: "nearest",
                  });
                }}
              >
                {state.tabs.map((tab) => {
                  const task =
                    tab.kind === "subagent"
                      ? subagentTabs.find((task) => task.taskId === tab.taskId)
                      : undefined;
                  const label =
                    tab.kind === "subagent"
                      ? (task?.description ?? "subagent")
                      : tab.kind === "artifact"
                        ? tab.selection.title
                        : (tab.tab?.name ??
                          (tab.mode === "browser" ? "浏览器" : "打开文件"));
                  const mediaIcon =
                    tab.kind === "artifact"
                      ? artifactPreviewTabMediaIconConcept(
                          tab.selection.mediaKind,
                        )
                      : undefined;
                  const iconKind = projectFileIconKind({
                    kind: "file",
                    name: label,
                  });
                  const selected =
                    state.activeId === tab.id &&
                    (tab.kind === "subagent" ||
                      files.fileWorkspaceTabsAreActive);
                  const icon =
                    tab.kind === "subagent" ? (
                      <AwakenIcon concept="subagent" size={16} />
                    ) : tab.kind === "file" && !tab.tab ? (
                      <AwakenIcon
                        concept={
                          tab.mode === "browser" ? "browser" : "file-generic"
                        }
                        size={16}
                      />
                    ) : (
                      <span
                        className="project-files-tab__file-icon"
                        data-file-icon={mediaIcon ?? iconKind}
                      >
                        {mediaIcon ? (
                          <AwakenIcon concept={mediaIcon} size={16} />
                        ) : (
                          <ProjectFileIcon kind={iconKind} size={16} />
                        )}
                      </span>
                    );
                  return (
                    <div
                      key={tab.id}
                      role="presentation"
                      className={`project-files-tab${tab.kind === "file" && !tab.tab ? " is-placeholder" : ""}${selected ? " is-active" : ""}`}
                    >
                      <button
                        type="button"
                        className="project-files-tab__activate"
                        role="tab"
                        aria-selected={selected}
                        title={label}
                        id={
                          tab.kind === "subagent"
                            ? `subagent-tab-${tab.taskId}`
                            : undefined
                        }
                        aria-controls={
                          tab.kind === "subagent"
                            ? `subagent-panel-${tab.taskId}`
                            : undefined
                        }
                        onClick={() =>
                          tab.kind === "subagent"
                            ? onOpenSubagent(tab.taskId)
                            : files.handleActivateInspectorTab(tab.id)
                        }
                      >
                        {icon}
                        <span className="project-files-tab__label">
                          {label}
                        </span>
                      </button>
                      <button
                        type="button"
                        className="project-files-tab__close"
                        aria-label={
                          tab.kind === "subagent"
                            ? `关闭子任务标签 ${label}`
                            : `关闭 ${label}`
                        }
                        title={`关闭 ${label}`}
                        onClick={() => dispatch({ type: "close", id: tab.id })}
                      >
                        <AwakenIcon concept="close" size={16} />
                      </button>
                    </div>
                  );
                })}
                {files.fileWorkspaceOpen &&
                files.showFileWorkspacePlaceholder ? (
                  <div
                    className={`project-files-tab is-placeholder${files.fileWorkspaceTabsAreActive ? " is-active" : ""}`}
                    role="presentation"
                  >
                    <button
                      type="button"
                      className="project-files-tab__activate"
                      role="tab"
                      aria-selected={files.fileWorkspaceTabsAreActive}
                      onClick={() => files.handleOpenFileWorkspace("files")}
                    >
                      <AwakenIcon concept="file-generic" size={16} />
                      <span className="project-files-tab__label">打开文件</span>
                    </button>
                    <button
                      type="button"
                      className="project-files-tab__close"
                      aria-label="关闭文件视图"
                      onClick={files.handleCloseFileWorkspace}
                    >
                      <AwakenIcon concept="close" size={16} />
                    </button>
                  </div>
                ) : null}
              </div>
              <Tooltip label="新建标签" placement="bottom">
                <button
                  type="button"
                  className="workbench-chrome-icon project-files-tab-add"
                  aria-label="新建标签"
                  onClick={files.handleAddProjectFileTab}
                >
                  <AwakenIcon concept="add" size={14} />
                </button>
              </Tooltip>
            </div>
            <div className="project-files-chrome-actions">
              {files.fileWorkspacePanelAction ? (
                <Tooltip
                  label={
                    files.fileWorkspacePanelAction === "contract"
                      ? "收起面板"
                      : "展开面板"
                  }
                  placement="bottom"
                >
                  <button
                    type="button"
                    className="workbench-chrome-icon workbench-chrome-expand-toggle"
                    aria-label={
                      files.fileWorkspacePanelAction === "contract"
                        ? "收起面板"
                        : "展开面板"
                    }
                    onClick={
                      files.fileWorkspacePanelAction === "contract"
                        ? files.handleContractFileWorkspace
                        : files.handleExpandFileWorkspace
                    }
                  >
                    <AwakenIcon
                      concept={
                        files.fileWorkspacePanelAction === "contract"
                          ? "exit-full"
                          : "expand-full"
                      }
                      size={14}
                    />
                  </button>
                </Tooltip>
              ) : null}
              {panelToggle}
            </div>
          </div>
        ) : (
          <div className="workbench-chrome-status">
            {files.inspectorPresentation.available ? panelToggle : null}
          </div>
        )
      }
    />
  );
}
