import type { Dispatch, SetStateAction, RefObject } from "react";
import type { useDesktopUpdate } from "./useDesktopUpdate";
import {
  AwakenIcon,
  AwakenSidebar as Sidebar,
  AwakenSidebarSectionAction as SidebarSectionAction,
  AwakenWorkbenchSidebarFrame as WorkbenchSidebarFrame,
  type NavItem,
  type ProjectItem,
} from "@awaken/design-system";
import { DEFAULT_WORKSPACE_PROJECT_ID } from "./projectWorkspace";
import { useProjectOperations } from "./useProjectOperations";
import { useProjectRegistry } from "./useProjectRegistry";
import { useThreadNavigation } from "./useThreadNavigation";
import { useBackgroundPrimaryClick } from "./useBackgroundPrimaryClick";
type Props = {
  onBackgroundPrimaryClick?: () => void;
  projectRegistry: Pick<
    ReturnType<typeof useProjectRegistry>,
    | "expandedProjectIds"
    | "setExpandedProjectIds"
    | "conversationSectionExpanded"
    | "setConversationSectionExpanded"
  >;
  projectOperations: Pick<
    ReturnType<typeof useProjectOperations>,
    | "projectReorderSubmitting"
    | "handleReorderProjects"
    | "handleOpenCreateProject"
  >;
  threadNavigation: Pick<
    ReturnType<typeof useThreadNavigation>,
    | "conversationBatchMode"
    | "setSelectedConversationThreadIds"
    | "conversationThreads"
    | "eligibleConversationThreadIds"
    | "selectedConversationThreadIdSet"
    | "selectedConversationCount"
    | "allEligibleConversationsSelected"
    | "conversationItems"
    | "enterConversationBatchMode"
    | "exitConversationBatchMode"
    | "requestThreadDelete"
  >;
  navItems: NavItem[];
  activeNavId: string | undefined;
  repositoryProjects: ProjectItem[];
  activeProjectId: string | undefined;
  allProjectsExpanded: boolean;
  handleNewTask: () => void;
  handleSelectNav: (id: string) => void;
  handleSelectProject: (id: string) => void;
  desktopReady: boolean;
  profilePopoverOpen: boolean;
  setProfilePopoverOpen: Dispatch<SetStateAction<boolean>>;
  profileButtonRef: RefObject<HTMLButtonElement | null>;
  sidebarUpdateAction: ReturnType<
    typeof useDesktopUpdate
  >["sidebarUpdateAction"];
};
export function DesktopSidebar({
  onBackgroundPrimaryClick,
  projectRegistry,
  projectOperations,
  threadNavigation,
  navItems,
  activeNavId,
  repositoryProjects,
  activeProjectId,
  allProjectsExpanded,
  handleNewTask,
  handleSelectNav,
  handleSelectProject,
  desktopReady,
  profilePopoverOpen,
  setProfilePopoverOpen,
  profileButtonRef,
  sidebarUpdateAction,
}: Props) {
  const backgroundClick = useBackgroundPrimaryClick(onBackgroundPrimaryClick);
  const {
    expandedProjectIds,
    setExpandedProjectIds,
    conversationSectionExpanded,
    setConversationSectionExpanded,
  } = projectRegistry;
  const {
    projectReorderSubmitting,
    handleReorderProjects,
    handleOpenCreateProject,
  } = projectOperations;
  const {
    conversationBatchMode,
    setSelectedConversationThreadIds,
    conversationThreads,
    eligibleConversationThreadIds,
    selectedConversationThreadIdSet,
    selectedConversationCount,
    allEligibleConversationsSelected,
    conversationItems,
    enterConversationBatchMode,
    exitConversationBatchMode,
    requestThreadDelete,
  } = threadNavigation;
  return (
    <WorkbenchSidebarFrame {...backgroundClick}>
      <Sidebar
        navItems={navItems}
        activeNavId={activeNavId}
        projects={repositoryProjects}
        conversations={conversationItems}
        activeProjectId={activeProjectId}
        expandedIds={expandedProjectIds}
        defaultExpandedIds={[DEFAULT_WORKSPACE_PROJECT_ID]}
        profileName="Nolan"
        profileAvatarFallback="AW"
        newTaskLabel="新建任务"
        projectSectionLabel="项目工作区"
        projectSectionActions={
          <span className="sidebar-section-actions">
            <SidebarSectionAction
              label={allProjectsExpanded ? "收起全部项目" : "展开全部项目"}
              onClick={() =>
                setExpandedProjectIds(
                  allProjectsExpanded
                    ? []
                    : repositoryProjects.map((project) => project.id),
                )
              }
            >
              {allProjectsExpanded ? (
                <AwakenIcon concept="collapse-all" size={16} />
              ) : (
                <AwakenIcon concept="expand-all" size={16} />
              )}
            </SidebarSectionAction>
            <SidebarSectionAction
              label="创建项目"
              onClick={handleOpenCreateProject}
            >
              <AwakenIcon concept="project-add" size={16} />
            </SidebarSectionAction>
          </span>
        }
        conversationSectionLabel={
          conversationBatchMode
            ? `对话 · 已选 ${selectedConversationCount}`
            : "对话"
        }
        conversationSectionExpanded={conversationSectionExpanded}
        conversationSectionActions={
          <span className="sidebar-section-actions">
            {conversationBatchMode ? (
              <>
                <SidebarSectionAction
                  label={
                    allEligibleConversationsSelected
                      ? "取消全选"
                      : "全选可删除对话"
                  }
                  disabled={eligibleConversationThreadIds.length === 0}
                  onClick={() => {
                    setSelectedConversationThreadIds(
                      allEligibleConversationsSelected
                        ? []
                        : eligibleConversationThreadIds,
                    );
                  }}
                >
                  <AwakenIcon concept="select-all" size={16} />
                </SidebarSectionAction>
                <SidebarSectionAction
                  label={
                    selectedConversationCount > 0
                      ? `删除 ${selectedConversationCount} 个对话`
                      : "请选择要删除的对话"
                  }
                  className="conversation-batch-delete-action"
                  disabled={selectedConversationCount === 0}
                  onClick={() => {
                    requestThreadDelete(
                      Array.from(selectedConversationThreadIdSet),
                      "batch",
                    );
                  }}
                >
                  <AwakenIcon concept="delete" size={16} />
                </SidebarSectionAction>
                <SidebarSectionAction
                  label="退出批量管理"
                  onClick={exitConversationBatchMode}
                >
                  <AwakenIcon concept="close" size={16} />
                </SidebarSectionAction>
              </>
            ) : (
              <>
                <SidebarSectionAction
                  label={conversationSectionExpanded ? "收起对话" : "展开对话"}
                  onClick={() =>
                    setConversationSectionExpanded((expanded) => !expanded)
                  }
                >
                  {conversationSectionExpanded ? (
                    <AwakenIcon concept="collapse-all" size={16} />
                  ) : (
                    <AwakenIcon concept="expand-all" size={16} />
                  )}
                </SidebarSectionAction>
                <SidebarSectionAction
                  label="批量管理对话"
                  disabled={conversationThreads.length === 0}
                  onClick={enterConversationBatchMode}
                >
                  <AwakenIcon concept="select-mode" size={16} />
                </SidebarSectionAction>
                <SidebarSectionAction label="新增对话" onClick={handleNewTask}>
                  <AwakenIcon concept="thread-new" size={16} />
                </SidebarSectionAction>
              </>
            )}
          </span>
        }
        showProjectCount={false}
        showConversationCount={false}
        onSelectNav={handleSelectNav}
        onSelectProject={handleSelectProject}
        onSelectConversation={handleSelectProject}
        onReorderProjects={(ids) => {
          void handleReorderProjects(ids);
        }}
        projectReorderDisabled={!desktopReady || projectReorderSubmitting}
        onExpandedChange={setExpandedProjectIds}
        onNewTask={handleNewTask}
        profileExpanded={profilePopoverOpen}
        profileControls="aw-profile-menu-panel"
        onProfileClick={() => setProfilePopoverOpen((open) => !open)}
        profileButtonRef={profileButtonRef}
        profileUpdateAction={sidebarUpdateAction}
      />
    </WorkbenchSidebarFrame>
  );
}
