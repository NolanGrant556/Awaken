import {
  AwakenIcon,
  AwakenTooltip as Tooltip,
  AwakenWorkbenchChrome as WorkbenchChrome,
  type PlatformKind,
} from "@awaken/design-system";
import ProjectFileIcon from "./ProjectFileIcon";
import { artifactPreviewTabMediaIconConcept } from "./artifactMediaPresentation";
import { projectFileIconKind } from "./projectFiles";
import { useFileWorkspaceSession } from "./useFileWorkspaceSession";

type Props = {
  fileWorkspace: Pick<
    ReturnType<typeof useFileWorkspaceSession>,
    | "fileWorkspaceOpen"
    | "expandedWorkspaceSurface"
    | "fileWorkspaceMode"
    | "artifactPreviewTabsState"
    | "deliverablePreview"
    | "fileWorkspaceExpandedActive"
    | "sidebarCollapsed"
    | "setSidebarCollapsed"
    | "inspectorCollapsed"
    | "inspectorPresentation"
    | "fileWorkspacePanelAction"
    | "handleToggleInspectorCollapsed"
    | "handleExpandFileWorkspace"
    | "handleContractFileWorkspace"
    | "handleShowConversationSurface"
    | "handleShowFileWorkspaceSurface"
    | "handleCloseFileWorkspace"
    | "handleAddProjectFileTab"
    | "handleActivateProjectFileTab"
    | "handleCloseProjectFileTab"
    | "handleActivateArtifactPreviewTab"
    | "handleCloseArtifactPreviewTab"
    | "fileWorkspaceProjectId"
    | "activeFileWorkspaceTabSlot"
    | "visibleFileWorkspaceTabSlots"
    | "showFileWorkspacePlaceholder"
    | "fileWorkspaceTabsAreActive"
  >;
  platform: PlatformKind;
  handleNewTask: () => void;
  currentConversationLabel: string;
};
export function DesktopWorkbenchChrome({
  fileWorkspace,
  platform,
  handleNewTask,
  currentConversationLabel,
}: Props) {
  const {
    fileWorkspaceOpen,
    expandedWorkspaceSurface,
    fileWorkspaceMode,
    artifactPreviewTabsState,
    deliverablePreview,
    fileWorkspaceExpandedActive,
    sidebarCollapsed,
    setSidebarCollapsed,
    inspectorCollapsed,
    inspectorPresentation,
    fileWorkspacePanelAction,
    handleToggleInspectorCollapsed,
    handleExpandFileWorkspace,
    handleContractFileWorkspace,
    handleShowConversationSurface,
    handleShowFileWorkspaceSurface,
    handleCloseFileWorkspace,
    handleAddProjectFileTab,
    handleActivateProjectFileTab,
    handleCloseProjectFileTab,
    handleActivateArtifactPreviewTab,
    handleCloseArtifactPreviewTab,
    fileWorkspaceProjectId,
    activeFileWorkspaceTabSlot,
    visibleFileWorkspaceTabSlots,
    showFileWorkspacePlaceholder,
    fileWorkspaceTabsAreActive,
  } = fileWorkspace;
  return (
    <WorkbenchChrome
      platform={platform}
      sidebarCollapsed={sidebarCollapsed}
      onToggleSidebar={() => setSidebarCollapsed((collapsed) => !collapsed)}
      onNewTask={handleNewTask}
      center={
        fileWorkspaceExpandedActive && inspectorPresentation.available ? (
          <div
            className="workbench-conversation-tabs"
            role="tablist"
            aria-label="当前会话"
          >
            <button
              type="button"
              className={`workbench-conversation-tab${
                expandedWorkspaceSurface === "conversation" ? " is-active" : ""
              }`}
              role="tab"
              aria-selected={expandedWorkspaceSurface === "conversation"}
              title={currentConversationLabel}
              onClick={handleShowConversationSurface}
            >
              <AwakenIcon concept="memory-feedback" size={16} />
              <span>{currentConversationLabel}</span>
            </button>
          </div>
        ) : undefined
      }
      right={
        fileWorkspaceOpen && inspectorPresentation.available ? (
          <div className="workbench-chrome-file-mode">
            <div className="project-files-tab-rail">
              <div
                className="project-files-tabs"
                role="tablist"
                aria-label="打开的文件"
              >
                {artifactPreviewTabsState.tabs.map((artifactTab) => {
                  const artifactSelection = artifactTab.selection;
                  const mediaIconConcept = artifactPreviewTabMediaIconConcept(
                    artifactSelection.mediaKind,
                  );
                  const iconKind = projectFileIconKind({
                    kind: "file",
                    name: artifactSelection.title,
                  });
                  return (
                    <div
                      key={artifactTab.id}
                      className={`project-files-tab${
                        deliverablePreview &&
                        artifactTab.id ===
                          artifactPreviewTabsState.activeTabId &&
                        fileWorkspaceTabsAreActive
                          ? " is-active"
                          : ""
                      }`}
                      role="presentation"
                    >
                      <button
                        type="button"
                        className="project-files-tab__activate"
                        role="tab"
                        aria-selected={
                          artifactTab.id ===
                            artifactPreviewTabsState.activeTabId &&
                          fileWorkspaceTabsAreActive
                        }
                        title={artifactSelection.title}
                        onClick={() =>
                          handleActivateArtifactPreviewTab(artifactTab)
                        }
                      >
                        <span
                          className="project-files-tab__file-icon"
                          data-file-icon={mediaIconConcept ?? iconKind}
                        >
                          {mediaIconConcept ? (
                            <AwakenIcon concept={mediaIconConcept} size={16} />
                          ) : (
                            <ProjectFileIcon kind={iconKind} size={16} />
                          )}
                        </span>
                        <span className="project-files-tab__label">
                          {artifactSelection.title}
                        </span>
                      </button>
                      <button
                        type="button"
                        className="project-files-tab__close"
                        aria-label={`关闭 ${artifactSelection.title}`}
                        title={`关闭 ${artifactSelection.title}`}
                        onClick={() =>
                          handleCloseArtifactPreviewTab(artifactTab.id)
                        }
                      >
                        <AwakenIcon concept="close" size={16} />
                      </button>
                    </div>
                  );
                })}
                {visibleFileWorkspaceTabSlots.map((slot) => {
                  const tab = slot.tab;
                  const active =
                    !deliverablePreview &&
                    fileWorkspaceTabsAreActive &&
                    slot.id === activeFileWorkspaceTabSlot?.id;
                  const iconKind = tab
                    ? projectFileIconKind({
                        kind: "file",
                        name: tab.name,
                      })
                    : "file";
                  const label = tab?.name ?? "打开文件";
                  return (
                    <div
                      key={slot.id}
                      className={`project-files-tab${tab ? "" : " is-placeholder"}${
                        active ? " is-active" : ""
                      }`}
                      role="presentation"
                    >
                      <button
                        type="button"
                        className="project-files-tab__activate"
                        role="tab"
                        aria-selected={active}
                        title={label}
                        onClick={() => handleActivateProjectFileTab(slot)}
                      >
                        <span
                          className="project-files-tab__file-icon"
                          data-file-icon={iconKind}
                        >
                          {tab ? (
                            <ProjectFileIcon kind={iconKind} size={16} />
                          ) : (
                            <AwakenIcon concept="file-generic" size={16} />
                          )}
                        </span>
                        <span className="project-files-tab__label">
                          {label}
                        </span>
                      </button>
                      <button
                        type="button"
                        className="project-files-tab__close"
                        aria-label={`关闭 ${label}`}
                        title={`关闭 ${label}`}
                        onClick={() => handleCloseProjectFileTab(slot.id)}
                      >
                        <AwakenIcon concept="close" size={16} />
                      </button>
                    </div>
                  );
                })}
                {showFileWorkspacePlaceholder ? (
                  <div
                    className={`project-files-tab is-placeholder${
                      fileWorkspaceTabsAreActive ? " is-active" : ""
                    }`}
                    role="presentation"
                  >
                    <button
                      type="button"
                      className="project-files-tab__activate"
                      role="tab"
                      aria-selected={fileWorkspaceTabsAreActive}
                      onClick={handleShowFileWorkspaceSurface}
                    >
                      <AwakenIcon concept="file-generic" size={16} />
                      <span className="project-files-tab__label">
                        {fileWorkspaceMode === "browser"
                          ? "浏览器"
                          : "打开文件"}
                      </span>
                    </button>
                    <button
                      type="button"
                      className="project-files-tab__close"
                      aria-label="关闭文件视图"
                      onClick={handleCloseFileWorkspace}
                    >
                      <AwakenIcon concept="close" size={16} />
                    </button>
                  </div>
                ) : null}
              </div>
              <Tooltip label="新建文件标签" placement="bottom">
                <button
                  type="button"
                  className="workbench-chrome-icon project-files-tab-add"
                  aria-label="新建文件标签"
                  disabled={!fileWorkspaceProjectId}
                  onClick={handleAddProjectFileTab}
                >
                  <AwakenIcon concept="add" size={14} />
                </button>
              </Tooltip>
            </div>
            <div className="project-files-chrome-actions">
              {fileWorkspacePanelAction ? (
                <Tooltip
                  label={
                    fileWorkspacePanelAction === "contract"
                      ? "收起面板"
                      : "展开面板"
                  }
                  placement="bottom"
                >
                  <button
                    type="button"
                    className="workbench-chrome-icon workbench-chrome-expand-toggle"
                    aria-label={
                      fileWorkspacePanelAction === "contract"
                        ? "收起面板"
                        : "展开面板"
                    }
                    onClick={
                      fileWorkspacePanelAction === "contract"
                        ? handleContractFileWorkspace
                        : handleExpandFileWorkspace
                    }
                  >
                    <AwakenIcon
                      concept={
                        fileWorkspacePanelAction === "contract"
                          ? "exit-full"
                          : "expand-full"
                      }
                      size={14}
                    />
                  </button>
                </Tooltip>
              ) : null}
              <button
                type="button"
                className="workbench-chrome-icon workbench-chrome-panel-toggle"
                aria-label={
                  inspectorCollapsed ? "Expand inspector" : "Collapse inspector"
                }
                onClick={handleToggleInspectorCollapsed}
              >
                {inspectorCollapsed ? (
                  <AwakenIcon concept="toggle-right-panel" size={14} />
                ) : (
                  <AwakenIcon concept="toggle-right-panel" size={14} />
                )}
              </button>
            </div>
          </div>
        ) : (
          <div className="workbench-chrome-status">
            {inspectorPresentation.available ? (
              <button
                type="button"
                className="workbench-chrome-icon workbench-chrome-panel-toggle"
                aria-label={
                  inspectorCollapsed ? "Expand inspector" : "Collapse inspector"
                }
                onClick={handleToggleInspectorCollapsed}
              >
                {inspectorCollapsed ? (
                  <AwakenIcon concept="toggle-right-panel" size={14} />
                ) : (
                  <AwakenIcon concept="toggle-right-panel" size={14} />
                )}
              </button>
            ) : null}
          </div>
        )
      }
    />
  );
}
