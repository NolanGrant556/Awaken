import "./AwakenSidebar.css";
import type { CSSProperties, ReactNode } from "react";
import type {
  NavItem,
  ProjectItem,
} from "../../types";
import { AwakenNewTaskButton } from "../AwakenNewTaskButton";
import { AwakenPrimaryNav } from "../AwakenPrimaryNav";
import { AwakenProjectTree } from "../AwakenProjectTree";
import { AwakenProfileButton } from "../AwakenProfileButton";
import {
  AwakenSidebarUpdateButton,
  type AwakenSidebarUpdateAction,
} from "./AwakenSidebarUpdateButton";

export type AwakenSidebarProps = {
  navItems?: NavItem[];
  activeNavId?: string;
  projects?: ProjectItem[];
  conversations?: ProjectItem[];
  activeProjectId?: string;
  expandedIds?: string[];
  defaultExpandedIds?: string[];
  profileName?: string;
  profileAvatarFallback?: string;
  profileAvatarSrc?: string;
  newTaskLabel?: ReactNode;
  projectSectionLabel?: string;
  showProjectCount?: boolean;
  projectSectionActions?: ReactNode;
  conversationSectionLabel?: string;
  showConversationCount?: boolean;
  conversationSectionActions?: ReactNode;
  conversationSectionExpanded?: boolean;
  profileSubtitle?: string;
  profileExpanded?: boolean;
  profileControls?: string;
  onSelectNav?: (id: string) => void;
  onSelectProject?: (id: string) => void;
  onSelectConversation?: (id: string) => void;
  onReorderProjects?: (ids: string[]) => void;
  projectReorderDisabled?: boolean;
  onNewTask?: () => void;
  onExpandedChange?: (ids: string[]) => void;
  onProfileClick?: () => void;
  profileButtonRef?: React.RefObject<HTMLButtonElement | null>;
  profileUpdateAction?: AwakenSidebarUpdateAction;
};

const sidebarStyle: CSSProperties = {
  display: "grid",
  gridTemplateRows: "auto auto minmax(0, 1fr) auto",
  width: "var(--aw-sidebar-width)",
  height: "100%",
  minWidth: 0,
  overflow: "hidden",
  background: "var(--aw-bg-nav)",
};

const topStyle: CSSProperties = {
  display: "grid",
  gap: 12,
  padding: "calc(var(--aw-sidebar-top-inset, 8px) + 4px) 14px 10px",
};

const scrollStyle: CSSProperties = {
  minHeight: 0,
  padding: "2px 0 14px",
  borderTop: "1px solid var(--aw-border-light)",
};

const sectionStyle: CSSProperties = {
  padding: "12px 14px 0",
};

const sectionHeadStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  minHeight: 28,
  padding: "0 8px 4px 10px",
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-sm)",
  lineHeight: "var(--aw-line-height-sm)",
  fontWeight: "var(--aw-font-weight-medium)",
};

const headCountStyle: CSSProperties = {
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-sm)",
};

const bottomStyle: CSSProperties = {
  display: "grid",
  gap: 8,
  padding: "8px 14px 12px",
  position: "relative",
};

export function AwakenSidebar({
  navItems = [],
  activeNavId,
  projects = [],
  conversations = [],
  activeProjectId,
  expandedIds,
  defaultExpandedIds,
  profileName = "",
  profileAvatarFallback = "",
  profileAvatarSrc,
  newTaskLabel = "新建任务",
  projectSectionLabel = "项目工作区",
  showProjectCount = true,
  projectSectionActions,
  conversationSectionLabel = "对话",
  showConversationCount = false,
  conversationSectionActions,
  conversationSectionExpanded = true,
  profileSubtitle,
  profileExpanded,
  profileControls,
  onSelectNav,
  onSelectProject,
  onSelectConversation,
  onReorderProjects,
  projectReorderDisabled,
  onNewTask,
  onExpandedChange,
  onProfileClick,
  profileButtonRef,
  profileUpdateAction,
}: AwakenSidebarProps) {
  const showProjects = projectSectionLabel !== "" || projects.length > 0;
  const showConversations =
    conversationSectionLabel !== "" || conversations.length > 0;

  return (
    <aside className="aw-sidebar" aria-label="Awaken 导航" style={sidebarStyle}>
      <div style={topStyle}>
        <AwakenNewTaskButton
          onClick={onNewTask}
        >
          {newTaskLabel}
        </AwakenNewTaskButton>
      </div>

      <AwakenPrimaryNav items={navItems} activeId={activeNavId} onSelect={onSelectNav} />

      <div className="aw-sidebar-scroll" style={scrollStyle}>
        {showProjects ? (
          <section style={sectionStyle}>
            <div style={sectionHeadStyle}>
              <span>{projectSectionLabel}</span>
              <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                {showProjectCount && projects.length > 0 ? (
                  <span style={headCountStyle}>{projects.length}</span>
                ) : null}
                {projectSectionActions}
              </span>
            </div>
            <AwakenProjectTree
              items={projects}
              activeId={activeProjectId}
              expandedIds={expandedIds}
              defaultExpandedIds={defaultExpandedIds}
              onExpandedChange={onExpandedChange}
              onSelect={onSelectProject}
              onReorder={onReorderProjects}
              reorderDisabled={projectReorderDisabled}
            />
          </section>
        ) : null}
        {showConversations ? (
          <section style={sectionStyle}>
            <div style={sectionHeadStyle}>
              <span>{conversationSectionLabel}</span>
              <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                {showConversationCount && conversations.length > 0 ? (
                  <span style={headCountStyle}>{conversations.length}</span>
                ) : null}
                {conversationSectionActions}
              </span>
            </div>
            {conversationSectionExpanded ? (
              <AwakenProjectTree
                items={conversations}
                activeId={activeProjectId}
                onSelect={onSelectConversation ?? onSelectProject}
                emptyLabel="暂无对话"
              />
            ) : null}
          </section>
        ) : null}
      </div>

      {profileName || profileAvatarFallback || profileAvatarSrc || onProfileClick || profileUpdateAction ? (
        <div className="aw-sidebar-profile" style={bottomStyle}>
          {profileUpdateAction ? (
            <AwakenSidebarUpdateButton action={profileUpdateAction} />
          ) : null}
          {profileName || profileAvatarFallback || profileAvatarSrc || onProfileClick ? (
            <AwakenProfileButton
              name={profileName}
              subtitle={profileSubtitle}
              avatarFallback={profileAvatarFallback}
              avatarSrc={profileAvatarSrc}
              expanded={profileExpanded}
              controls={profileControls}
              onClick={onProfileClick}
              buttonRef={profileButtonRef}
            />
          ) : null}
        </div>
      ) : null}
    </aside>
  );
}
