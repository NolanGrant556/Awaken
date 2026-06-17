import type { CSSProperties } from "react";
import type {
  ConversationItem,
  NavItem,
  PlatformKind,
  ProjectItem,
} from "../../types";
import { BrandRow } from "./BrandRow";
import { NewTaskButton } from "./NewTaskButton";
import { PrimaryNav } from "./PrimaryNav";
import { ConversationList } from "./ConversationList";
import { ProjectTree } from "./ProjectTree";
import { ProfileButton } from "./ProfileButton";

type SidebarProps = {
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
  platform?: PlatformKind;
  onSelectNav?: (id: string) => void;
  onSelectConversation?: (id: string) => void;
  onSelectProject?: (id: string) => void;
  onNewTask?: () => void;
  onExpandedChange?: (ids: string[]) => void;
  onProfileClick?: () => void;
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
  gap: 10,
  padding: "12px 8px 8px",
};

const scrollStyle: CSSProperties = {
  minHeight: 0,
  paddingBottom: 12,
};

const sectionStyle: CSSProperties = {
  padding: "10px 8px 0",
};

const sectionHeadStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  minHeight: 28,
  padding: "0 8px",
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
};

const headCountStyle: CSSProperties = {
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
};

const bottomStyle: CSSProperties = {
  display: "grid",
  gap: 8,
  padding: 8,
};

export function Sidebar({
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
  platform = "macos",
  onSelectNav,
  onSelectConversation,
  onSelectProject,
  onNewTask,
  onExpandedChange,
  onProfileClick,
}: SidebarProps) {
  return (
    <aside className="aw-sidebar" aria-label="Awaken 导航" style={sidebarStyle}>
      <div style={topStyle}>
        <BrandRow title={title} />
        <NewTaskButton
          onClick={onNewTask}
          shortcut={newTaskShortcut}
          platform={platform}
        />
      </div>

      <PrimaryNav items={navItems} activeId={activeNavId} onSelect={onSelectNav} />

      <div className="aw-scroll-hidden" style={scrollStyle}>
        <section style={sectionStyle}>
          <div style={sectionHeadStyle}>
            <span>对话</span>
          </div>
          <ConversationList
            items={conversations}
            activeId={activeConversationId}
            onSelect={onSelectConversation}
          />
        </section>

        <section style={sectionStyle}>
          <div style={sectionHeadStyle}>
            <span>项目</span>
            {projects.length > 0 ? (
              <span style={headCountStyle}>{projects.length}</span>
            ) : null}
          </div>
          <ProjectTree
            items={projects}
            activeId={activeProjectId}
            expandedIds={expandedIds}
            defaultExpandedIds={defaultExpandedIds}
            onExpandedChange={onExpandedChange}
            onSelect={onSelectProject}
          />
        </section>
      </div>

      {profileName || profileAvatarFallback || profileAvatarSrc || onProfileClick ? (
        <div style={bottomStyle}>
          <ProfileButton
            name={profileName}
            avatarFallback={profileAvatarFallback}
            avatarSrc={profileAvatarSrc}
            onClick={onProfileClick}
          />
        </div>
      ) : null}
    </aside>
  );
}
