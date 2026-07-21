import { createElement, type ComponentProps } from "react";
import { FolderPlus, ListFilter, MessageCirclePlus, Plug, Wrench } from "lucide-react";
import { AwakenSidebarSectionAction } from "../AwakenSidebarSectionAction";
import type { AwakenSidebar } from "./AwakenSidebar";

export type AwakenSidebarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSidebar>>;
};

const noop = () => {};

function projectAction(label: string) {
  return createElement(
    AwakenSidebarSectionAction,
    {
      label: `New session in ${label}`,
      children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
    },
  );
}

function projectMenuItems() {
  return [
    { id: "remove-project", label: "从 Awaken 移除", onSelect: noop },
    { id: "delete-project-history", label: "删除 Awaken 历史", tone: "danger" as const, onSelect: noop },
    { id: "trash-project", label: "移到废纸篓", tone: "danger" as const, onSelect: noop },
  ];
}

function threadMenuItems() {
  return [
    { id: "rename-thread", label: "重命名", onSelect: noop },
    { id: "delete-thread", label: "删除会话", tone: "danger" as const, onSelect: noop },
  ];
}

export const AwakenSidebarStates: AwakenSidebarState[] = [
  {
    id: "current-desktop-sidebar",
    label: "Awaken 当前侧栏",
    props: {
      navItems: [
        {
          id: "connectors",
          label: "连接器",
          icon: createElement(Plug, { size: 16, strokeWidth: 1.9 }),
        },
        {
          id: "skills",
          label: "技能",
          icon: createElement(Wrench, { size: 16, strokeWidth: 1.9 }),
        },
      ],
      activeNavId: "skills",
      projects: [
        {
          id: "management",
          label: "09_经营管理部",
          kind: "project",
          path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/09_经营管理部",
          action: projectAction("09_经营管理部"),
          menuItems: projectMenuItems(),
          children: [
            { id: "weekly-review", kind: "thread", label: "管理层周报", menuItems: threadMenuItems() },
          ],
        },
        {
          id: "legal",
          label: "08_行政与法务部",
          kind: "project",
          path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/08_行政与法务部",
          action: projectAction("08_行政与法务部"),
          menuItems: projectMenuItems(),
          children: [
            { id: "contract-check", kind: "thread", label: "合同条款检查", menuItems: threadMenuItems() },
          ],
        },
        {
          id: "service",
          label: "07_客户服务部",
          kind: "project",
          path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/07_客户服务部",
          action: projectAction("07_客户服务部"),
          menuItems: projectMenuItems(),
          children: [
            { id: "service-summary", kind: "thread", label: "客户问题汇总", menuItems: threadMenuItems() },
          ],
        },
        {
          id: "finance",
          label: "01_财务部",
          kind: "project",
          path: "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/01_财务部",
          action: projectAction("01_财务部"),
          menuItems: projectMenuItems(),
          children: [
            {
              id: "finance-attachment",
              kind: "thread",
              label: "项目附件确认",
              isLoading: true,
              menuItems: threadMenuItems(),
            },
          ],
        },
      ],
      activeProjectId: "finance-attachment",
      expandedIds: ["finance"],
      defaultExpandedIds: ["finance"],
      profileName: "Nolan",
      profileAvatarFallback: "AW",
      profileAvatarSrc: undefined,
      newTaskShortcut: "",
      newTaskLabel: "New Agent",
      projectSectionLabel: "Projects",
      showProjectCount: false,
      projectSectionActions: createElement(
        "span",
        { style: { display: "inline-flex", alignItems: "center", gap: 2 } },
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "Filter projects",
            children: createElement(ListFilter, { size: 13, strokeWidth: 1.85 }),
          },
        ),
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "Add folder",
            children: createElement(FolderPlus, { size: 13, strokeWidth: 1.85 }),
          },
        ),
      ),
      profileSubtitle: undefined,
      platform: "macos",
      onSelectNav: noop,
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
      navItems: [
        { id: "runs", label: "运行" },
        { id: "settings", label: "设置" },
      ],
      activeNavId: "settings",
      projects: [],
      activeProjectId: undefined,
      expandedIds: [],
      defaultExpandedIds: [],
      profileName: "",
      profileAvatarFallback: "",
      profileAvatarSrc: undefined,
      newTaskShortcut: "N",
      newTaskLabel: "新建任务",
      projectSectionLabel: "",
      showProjectCount: false,
      projectSectionActions: null,
      profileSubtitle: undefined,
      platform: "macos",
      onSelectNav: noop,
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
      navItems: [
        { id: "runs", label: "运行" },
        { id: "integrations", label: "集成", disabled: true },
        { id: "settings", label: "设置" },
      ],
      activeNavId: "runs",
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
      projectSectionLabel: "项目",
      showProjectCount: true,
      projectSectionActions: null,
      profileSubtitle: "正在扫描工作区",
      platform: "macos",
      onSelectNav: noop,
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
      navItems: [
        { id: "context", label: "当前工作区上下文与权限记录", badge: 5 },
        { id: "runtime", label: "本机运行时状态" },
      ],
      activeNavId: "context",
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
      platform: "macos",
      onSelectNav: noop,
      onSelectProject: noop,
      onNewTask: noop,
      onExpandedChange: noop,
      onProfileClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
];
