import { createElement, type ComponentProps } from "react";
import { FolderPlus, ListFilter, MessageCirclePlus } from "lucide-react";
import { AwakenSidebarSectionAction } from "../AwakenSidebarSectionAction";
import type { AwakenSidebar } from "./AwakenSidebar";

export type AwakenSidebarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSidebar>>;
};

const noop = () => {};

export const AwakenSidebarStates: AwakenSidebarState[] = [
  {
    id: "desktop-sidebar",
    label: "完整侧边栏",
    props: {
      title: "Awaken",
      navItems: [
        { id: "runs", label: "运行", badge: 2 },
        { id: "projects", label: "项目" },
        { id: "settings", label: "设置" },
      ],
      activeNavId: "runs",
      conversations: [
        {
          id: "design-system-docs",
          label: "补齐组件用途说明",
          timeLabel: "now",
          statusLabel: "进行中",
          statusTone: "local",
        },
        {
          id: "runtime-smoke",
          label: "桌面运行 smoke",
          timeLabel: "1h",
          statusLabel: "已完成",
          statusTone: "success",
        },
      ],
      activeConversationId: "design-system-docs",
      projects: [
        {
          id: "awaken",
          label: "Awaken",
          kind: "project",
          timeLabel: "当前",
          action: createElement(
            AwakenSidebarSectionAction,
            {
              label: "新建会话",
              children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
            },
          ),
          children: [
            { id: "group-3", kind: "thread", label: "Design System 第 3 组", timeLabel: "16" },
            { id: "desktop", kind: "thread", label: "apps/desktop", timeLabel: "8" },
          ],
        },
      ],
      activeProjectId: "group-3",
      expandedIds: ["awaken"],
      defaultExpandedIds: ["awaken"],
      profileName: "Nolan Grant",
      profileAvatarFallback: "NG",
      profileAvatarSrc: undefined,
      newTaskShortcut: "N",
      newTaskLabel: "新建任务",
      conversationSectionLabel: "最近任务",
      projectSectionLabel: "项目",
      showProjectCount: true,
      projectSectionActions: createElement(
        "span",
        { style: { display: "inline-flex", alignItems: "center", gap: 2 } },
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "筛选项目",
            children: createElement(ListFilter, { size: 13, strokeWidth: 1.85 }),
          },
        ),
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "添加文件夹",
            children: createElement(FolderPlus, { size: 13, strokeWidth: 1.85 }),
          },
        ),
      ),
      profileSubtitle: "本机运行时已连接",
      showBrand: true,
      platform: "macos",
      onSelectNav: noop,
      onSelectConversation: noop,
      onSelectProject: noop,
      onNewTask: noop,
      onExpandedChange: noop,
      onProfileClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
  {
    id: "minimal",
    label: "精简导航",
    props: {
      title: "Awaken",
      navItems: [
        { id: "runs", label: "运行" },
        { id: "settings", label: "设置" },
      ],
      activeNavId: "settings",
      conversations: [],
      activeConversationId: undefined,
      projects: [],
      activeProjectId: undefined,
      expandedIds: [],
      defaultExpandedIds: [],
      profileName: "",
      profileAvatarFallback: "",
      profileAvatarSrc: undefined,
      newTaskShortcut: "N",
      newTaskLabel: "新建任务",
      conversationSectionLabel: "",
      projectSectionLabel: "",
      showProjectCount: false,
      projectSectionActions: null,
      profileSubtitle: undefined,
      showBrand: false,
      platform: "macos",
      onSelectNav: noop,
      onSelectConversation: noop,
      onSelectProject: noop,
      onNewTask: noop,
      onExpandedChange: noop,
      onProfileClick: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
  {
    id: "loading-project",
    label: "项目加载中",
    props: {
      title: "Awaken",
      navItems: [
        { id: "runs", label: "运行" },
        { id: "integrations", label: "集成", disabled: true },
        { id: "settings", label: "设置" },
      ],
      activeNavId: "runs",
      conversations: [
        { id: "handoff", label: "继续上次任务", timeLabel: "昨天", statusLabel: "待继续" },
      ],
      activeConversationId: "handoff",
      projects: [
        {
          id: "workspace",
          label: "当前工作区",
          kind: "project",
          children: [
            { id: "scan", kind: "thread", label: "正在读取项目目录", isLoading: true },
            { id: "archive", kind: "thread", label: "归档项目", disabled: true },
          ],
        },
      ],
      activeProjectId: "scan",
      expandedIds: ["workspace"],
      defaultExpandedIds: ["workspace"],
      profileName: "Nolan Grant",
      profileAvatarFallback: "NG",
      profileAvatarSrc: undefined,
      newTaskShortcut: "N",
      newTaskLabel: "新建任务",
      conversationSectionLabel: "最近任务",
      projectSectionLabel: "项目",
      showProjectCount: true,
      projectSectionActions: null,
      profileSubtitle: "正在扫描工作区",
      showBrand: true,
      platform: "macos",
      onSelectNav: noop,
      onSelectConversation: noop,
      onSelectProject: noop,
      onNewTask: noop,
      onExpandedChange: noop,
      onProfileClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
  {
    id: "long-labels",
    label: "长文案",
    props: {
      title: "Awaken 本机桌面运行工作区",
      navItems: [
        { id: "context", label: "当前工作区上下文与权限记录", badge: 5 },
        { id: "runtime", label: "本机运行时状态" },
      ],
      activeNavId: "context",
      conversations: [
        {
          id: "long-task",
          label: "围绕第 3 组设计系统组件补齐用途说明和状态示例",
          timeLabel: "刚刚",
          statusLabel: "文档整理中",
        },
      ],
      activeConversationId: "long-task",
      projects: [
        {
          id: "long-project",
          label: "包含多个本地 runtime 与设计系统验收任务的 Awaken 工作区",
          kind: "project",
          path: "/Users/nolangrant/Awaken",
          timeLabel: "当前",
          children: [
            { id: "long-child", kind: "thread", label: "Awaken Design System/src/components 第 3 组", timeLabel: "16" },
          ],
        },
      ],
      activeProjectId: "long-child",
      expandedIds: ["long-project"],
      defaultExpandedIds: ["long-project"],
      profileName: "Nolan Grant",
      profileAvatarFallback: "NG",
      profileAvatarSrc: undefined,
      newTaskShortcut: "N",
      newTaskLabel: "新建带当前上下文的任务",
      conversationSectionLabel: "最近任务",
      projectSectionLabel: "当前项目与组件分组",
      showProjectCount: true,
      projectSectionActions: createElement(
        AwakenSidebarSectionAction,
        {
          label: "添加文件夹",
          children: createElement(FolderPlus, { size: 13, strokeWidth: 1.85 }),
        },
      ),
      profileSubtitle: "/Users/nolangrant/Awaken",
      showBrand: true,
      platform: "macos",
      onSelectNav: noop,
      onSelectConversation: noop,
      onSelectProject: noop,
      onNewTask: noop,
      onExpandedChange: noop,
      onProfileClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
];
