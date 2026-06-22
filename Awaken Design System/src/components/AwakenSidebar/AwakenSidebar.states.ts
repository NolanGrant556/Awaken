import type { ComponentProps } from "react";
import type { AwakenSidebar } from "./AwakenSidebar";

export type AwakenSidebarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSidebar>>;
};

const noop = () => {};

export const AwakenSidebarStates: AwakenSidebarState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenSidebar",
      navItems: [{ id: "new", label: "New Agent" }],
      activeNavId: "default",
      conversations: [{ id: "thread", title: "Component review", timeLabel: "now" }],
      activeConversationId: "default",
      projects: [{ id: "awaken", label: "awaken" }],
      activeProjectId: "default",
      expandedIds: ["awaken"],
      defaultExpandedIds: ["awaken"],
      profileName: "AwakenSidebar",
      profileAvatarFallback: "AwakenSidebar",
      profileAvatarSrc: "AwakenSidebar",
      newTaskShortcut: "AwakenSidebar",
      newTaskLabel: "AwakenSidebar",
      conversationSectionLabel: "AwakenSidebar",
      projectSectionLabel: "AwakenSidebar",
      showProjectCount: true,
      projectSectionActions: "AwakenSidebar",
      profileSubtitle: "AwakenSidebar",
      showBrand: true,
      platform: "macos",
      onSelectNav: noop,
      onSelectConversation: noop,
      onSelectProject: noop,
      onNewTask: noop,
      onExpandedChange: noop,
      onProfileClick: noop,
      children: "AwakenSidebar",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenSidebar with a deliberately long value that should remain readable inside its container",
      children: "AwakenSidebar with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
];
