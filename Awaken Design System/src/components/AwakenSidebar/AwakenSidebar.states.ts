import { createElement, type ComponentProps } from "react";
import {
  ChevronsDownUp,
  ChevronsUpDown,
  FolderPlus,
  MessageCirclePlus,
  Plug,
  Wrench,
} from "lucide-react";
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

// 与 Desktop 项目行菜单一致。废纸篓已经下沉到文件夹粒度，不在项目层出现。
function projectMenuItems() {
  return [
    { id: "add-project-folder", label: "添加文件夹", onSelect: noop },
    { id: "edit-project", label: "编辑项目", onSelect: noop },
    { id: "remove-project", label: "从 Awaken 移除", onSelect: noop },
    { id: "delete-project-history", label: "删除 Awaken 历史", tone: "danger" as const, onSelect: noop },
  ];
}

function projectHoverCard(title: string, summary: string, folders: string[]) {
  return {
    title,
    summary,
    folders: folders.map((path) => ({
      path,
      label: path.split("/").pop() ?? path,
    })),
    emptyFoldersLabel: "这个项目还没有文件夹",
    revealHint: "在文件管理器中显示：",
    actionLabel: "编辑项目",
    onRevealFolder: noop,
    onAction: noop,
  };
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
          hoverCard: projectHoverCard("09_经营管理部", "1 个对话串", [
            "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/09_经营管理部",
          ]),
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
          hoverCard: projectHoverCard("01_财务部", "1 个对话串", [
            "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/01_财务部/deliverables",
            "/Users/nolangrant/Awaken 测试集/Nolan实业有限公司/01_财务部/typst-work",
          ]),
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
      conversations: [
        { id: "conversation-1", kind: "thread", label: "梳理今天的工作重点", menuItems: threadMenuItems() },
        { id: "conversation-2", kind: "thread", label: "比较两个实施方案", menuItems: threadMenuItems() },
        { id: "conversation-3", kind: "thread", label: "整理会议笔记", menuItems: threadMenuItems() },
        { id: "conversation-4", kind: "thread", label: "生成一份说明文档", menuItems: threadMenuItems() },
        { id: "conversation-5", kind: "thread", label: "检查交付清单", menuItems: threadMenuItems() },
        { id: "conversation-6", kind: "thread", label: "分析临时数据", isLoading: true, menuItems: threadMenuItems() },
        { id: "conversation-7", kind: "thread", label: "规划下周任务", menuItems: threadMenuItems() },
      ],
      activeProjectId: "finance-attachment",
      expandedIds: ["finance"],
      defaultExpandedIds: ["finance"],
      profileName: "Nolan",
      profileAvatarFallback: "AW",
      profileAvatarSrc: undefined,
      newTaskLabel: "New Agent",
      projectSectionLabel: "项目工作区",
      showProjectCount: false,
      projectSectionActions: createElement(
        "span",
        { style: { display: "inline-flex", alignItems: "center", gap: 2 } },
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "展开全部项目",
            children: createElement(ChevronsUpDown, { size: 13, strokeWidth: 1.85 }),
          },
        ),
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "创建项目",
            children: createElement(FolderPlus, { size: 13, strokeWidth: 1.85 }),
          },
        ),
      ),
      conversationSectionLabel: "对话",
      conversationSectionExpanded: true,
      conversationSectionActions: createElement(
        "span",
        { style: { display: "inline-flex", alignItems: "center", gap: 2 } },
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "收起对话",
            children: createElement(ChevronsDownUp, { size: 13, strokeWidth: 1.85 }),
          },
        ),
        createElement(
          AwakenSidebarSectionAction,
          {
            label: "新增对话",
            children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
          },
        ),
      ),
      profileSubtitle: undefined,
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
      newTaskLabel: "新建任务",
      projectSectionLabel: "",
      conversationSectionLabel: "",
      showProjectCount: false,
      projectSectionActions: null,
      profileSubtitle: undefined,
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
      newTaskLabel: "新建任务",
      projectSectionLabel: "项目",
      conversationSectionLabel: "",
      showProjectCount: true,
      projectSectionActions: null,
      profileSubtitle: "正在扫描工作区",
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
      newTaskLabel: "新建带当前上下文的任务",
      projectSectionLabel: "当前项目与组件分组",
      conversationSectionLabel: "",
      showProjectCount: true,
      projectSectionActions: createElement(
        AwakenSidebarSectionAction,
        {
          label: "添加文件夹",
          children: createElement(FolderPlus, { size: 13, strokeWidth: 1.85 }),
        },
      ),
      profileSubtitle: "/Users/nolangrant/Awaken",
      onSelectNav: noop,
      onSelectProject: noop,
      onNewTask: noop,
      onExpandedChange: noop,
      onProfileClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenSidebar>>,
  },
  {
    id: "update-available",
    label: "有可用更新",
    props: {
      profileName: "Nolan",
      profileAvatarFallback: "AW",
      onProfileClick: noop,
      profileUpdateAction: { state: "available", label: "更新到 v0.2.0", onClick: noop },
    },
  },
  {
    id: "update-downloading-indeterminate",
    label: "更新下载中（未知总量）",
    props: {
      profileName: "Nolan",
      profileAvatarFallback: "AW",
      onProfileClick: noop,
      profileUpdateAction: { state: "downloading", label: "正在下载更新…", onClick: noop },
    },
  },
  {
    id: "update-downloading-37",
    label: "更新下载 37%",
    props: {
      profileName: "Nolan",
      profileAvatarFallback: "AW",
      onProfileClick: noop,
      profileUpdateAction: { state: "downloading", label: "正在下载更新", progress: 37, onClick: noop },
    },
  },
  {
    id: "update-downloaded",
    label: "更新已下载待重启",
    props: {
      profileName: "Nolan",
      profileAvatarFallback: "AW",
      onProfileClick: noop,
      profileUpdateAction: { state: "downloaded", label: "重启以完成更新", onClick: noop },
    },
  },
  {
    id: "update-error",
    label: "更新失败",
    props: {
      profileName: "Nolan",
      profileAvatarFallback: "AW",
      onProfileClick: noop,
      profileUpdateAction: { state: "error", label: "更新失败，点击重试", onClick: noop },
    },
  },
];
