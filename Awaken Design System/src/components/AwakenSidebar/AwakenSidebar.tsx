import "./AwakenSidebar.css";
import type { CSSProperties, ReactNode } from "react";
import type {
  ConversationItem,
  NavItem,
  PlatformKind,
  ProjectItem,
} from "../../types";
import { AwakenBrandRow } from "../AwakenBrandRow";
import { AwakenNewTaskButton } from "../AwakenNewTaskButton";
import { AwakenPrimaryNav } from "../AwakenPrimaryNav";
import { AwakenConversationList } from "../AwakenConversationList";
import { AwakenProjectTree } from "../AwakenProjectTree";
import { AwakenProfileButton } from "../AwakenProfileButton";

export type AwakenSidebarProps = {
  title?: string;
  navItems?: NavItem[];
  activeNavId?: string;
  conversations?: ConversationItem[];
  activeConversationId?: string;
  projects?: ProjectItem[];
  activeProjectId?: string;
  expandedIds?: string[];
  defaultExpandedIds?: string[];
  profileName?: string;
  profileAvatarFallback?: string;
  profileAvatarSrc?: string;
  newTaskShortcut?: string;
  newTaskLabel?: ReactNode;
  conversationSectionLabel?: string;
  projectSectionLabel?: string;
  showProjectCount?: boolean;
  projectSectionActions?: ReactNode;
  profileSubtitle?: string;
  showBrand?: boolean;
  platform?: PlatformKind;
  onSelectNav?: (id: string) => void;
  onSelectConversation?: (id: string) => void;
  onSelectProject?: (id: string) => void;
  onNewTask?: () => void;
  onExpandedChange?: (ids: string[]) => void;
  onProfileClick?: () => void;
  profileButtonRef?: React.RefObject<HTMLDivElement | null>;
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
  padding: "16px 14px 10px",
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
  minHeight: 24,
  padding: "0 6px 4px",
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
  fontWeight: "var(--aw-font-weight-medium)",
};

const headCountStyle: CSSProperties = {
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
};

const bottomStyle: CSSProperties = {
  display: "grid",
  gap: 8,
  padding: "8px 14px 12px",
  position: "relative",
};

export function AwakenSidebar({
  title = "Awaken",
  navItems = [],
  activeNavId,
  conversations = [],
  activeConversationId,
  projects = [],
  activeProjectId,
  expandedIds,
  defaultExpandedIds,
  profileName = "",
  profileAvatarFallback = "",
  profileAvatarSrc,
  newTaskShortcut = "N",
  newTaskLabel = "新建任务",
  conversationSectionLabel = "对话",
  projectSectionLabel = "项目",
  showProjectCount = true,
  projectSectionActions,
  profileSubtitle,
  showBrand = true,
  platform = "macos",
  onSelectNav,
  onSelectConversation,
  onSelectProject,
  onNewTask,
  onExpandedChange,
  onProfileClick,
  profileButtonRef,
}: AwakenSidebarProps) {
  const showConversations = conversationSectionLabel !== "" || conversations.length > 0;
  const showProjects = projectSectionLabel !== "" || projects.length > 0;

  return (
    <aside className="aw-sidebar" aria-label="Awaken 导航" style={sidebarStyle}>
      <div
        style={{
          ...topStyle,
          padding: showBrand
            ? topStyle.padding
            : "calc(var(--aw-sidebar-top-inset, 8px) + 4px) 14px 10px",
        }}
      >
        {showBrand ? <AwakenBrandRow title={title} /> : null}
        <AwakenNewTaskButton
          onClick={onNewTask}
          shortcut={newTaskShortcut}
          platform={platform}
        >
          {newTaskLabel}
        </AwakenNewTaskButton>
      </div>

      <AwakenPrimaryNav items={navItems} activeId={activeNavId} onSelect={onSelectNav} />

      <div className="aw-scroll-hidden" style={scrollStyle}>
        {showConversations ? (
          <section style={sectionStyle}>
            <div style={sectionHeadStyle}>
              <span>{conversationSectionLabel}</span>
            </div>
            <AwakenConversationList
              items={conversations}
              activeId={activeConversationId}
              onSelect={onSelectConversation}
            />
          </section>
        ) : null}

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
            />
          </section>
        ) : null}
      </div>

      {profileName || profileAvatarFallback || profileAvatarSrc || onProfileClick ? (
        <div ref={profileButtonRef} className="aw-sidebar-profile" style={bottomStyle}>
          <AwakenProfileButton
            name={profileName}
            subtitle={profileSubtitle}
            avatarFallback={profileAvatarFallback}
            avatarSrc={profileAvatarSrc}
            onClick={onProfileClick}
          />
        </div>
      ) : null}
    </aside>
  );
}
