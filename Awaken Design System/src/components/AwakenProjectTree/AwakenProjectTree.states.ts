import { createElement, type ComponentProps } from "react";
import { MessageCirclePlus } from "../../icons";
import { AwakenSidebarSectionAction } from "../AwakenSidebarSectionAction";
import type { AwakenProjectTree } from "./AwakenProjectTree";

export type AwakenProjectTreeState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenProjectTree>>;
};

const noop = () => {};

export const AwakenProjectTreeStates: AwakenProjectTreeState[] = [
  {
    id: "expanded-workspace",
    label: "展开项目",
    props: {
      items: [
        {
          id: "awaken",
          label: "Awaken",
          kind: "project",
          action: createElement(
            AwakenSidebarSectionAction,
            {
              label: "新建会话",
              children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
            },
          ),
          menuItems: [
            { id: "remove-folder", label: "移除文件夹" },
            { id: "delete-folder-history", label: "全量删除历史会话", tone: "danger" },
          ],
          children: [
            {
              id: "design-system",
              kind: "thread",
              label: "Awaken Design System",
              timeLabel: "16",
              menuItems: [
                { id: "rename-thread", label: "重命名" },
                { id: "delete-thread", label: "删除会话", tone: "danger" },
              ],
            },
            { id: "desktop", kind: "thread", label: "apps/desktop", timeLabel: "8" },
          ],
        },
      ],
      activeId: "design-system",
      defaultExpandedIds: ["awaken"],
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
  {
    id: "empty-project-expanded",
    label: "空项目已展开",
    props: {
      items: [
        {
          id: "empty-awaken",
          label: "Awaken",
          kind: "project",
          path: "/Users/nolangrant/Awaken",
          action: createElement(
            AwakenSidebarSectionAction,
            {
              label: "新建会话",
              children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
            },
          ),
        },
      ],
      activeId: "empty-awaken",
      defaultExpandedIds: ["empty-awaken"],
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
  {
    id: "hover-card-folders",
    label: "悬停展示文件夹组成",
    props: {
      items: [
        {
          id: "multi-folder",
          label: "Awaken 开发",
          kind: "project",
          path: "/Users/nolangrant/Awaken",
          action: createElement(
            AwakenSidebarSectionAction,
            {
              label: "新建会话",
              children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
            },
          ),
          menuItems: [
            { id: "add-project-folder", label: "添加文件夹" },
            { id: "edit-project", label: "编辑项目" },
          ],
          hoverCard: {
            title: "Awaken 开发",
            summary: "3 个对话串",
            folders: [
              { path: "/Users/nolangrant/Awaken", label: "Awaken" },
              { path: "/Users/nolangrant/Codex 源代码", label: "Codex 源代码" },
              { path: "/Users/nolangrant/claudecode源代码", label: "claudecode源代码" },
            ],
            revealHint: "在文件管理器中显示：",
            actionLabel: "编辑项目",
            onRevealFolder: noop,
            onAction: noop,
          },
        },
        {
          id: "no-folder",
          label: "只聊天的项目",
          kind: "project",
          hoverCard: {
            title: "只聊天的项目",
            summary: "暂无对话串",
            folders: [],
            emptyFoldersLabel: "这个项目还没有文件夹",
            actionLabel: "编辑项目",
            onAction: noop,
          },
        },
      ],
      activeId: "multi-folder",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
  {
    id: "loading-node",
    label: "节点加载中",
    props: {
      items: [
        {
          id: "workspace",
          label: "当前工作区",
          kind: "project",
          children: [
            { id: "scan-components", kind: "thread", label: "正在扫描组件目录", isLoading: true },
            { id: "docs", kind: "thread", label: "docs", disabled: true },
          ],
        },
      ],
      activeId: "scan-components",
      defaultExpandedIds: ["workspace"],
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
  {
    id: "empty",
    label: "无项目",
    props: {
      items: [],
      activeId: undefined,
      expandedIds: [],
      defaultExpandedIds: [],
      onExpandedChange: noop,
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
  {
    id: "long-label",
    label: "长项目名",
    props: {
      items: [
        {
          id: "long-project",
          label: "包含多个本地 runtime 与设计系统验收任务的 Awaken 工作区",
          kind: "project",
          path: "/Users/nolangrant/Awaken",
          children: [
            {
              id: "long-child",
              kind: "thread",
              label: "第 3 组组件用途说明与真实状态示例",
              timeLabel: "进行中",
            },
          ],
        },
      ],
      activeId: "long-child",
      defaultExpandedIds: ["long-project"],
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
  {
    id: "sdk-folder-name",
    label: "SDK 项目名",
    props: {
      items: [
        {
          id: "sdk-folder",
          label: "OpenAI_Agents_SDK_TS",
          kind: "project",
          path: "/Users/nolangrant/Documents/OpenAI_Agents_SDK_TS",
          action: createElement(
            AwakenSidebarSectionAction,
            {
              label: "新建会话",
              children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
            },
          ),
        },
      ],
      activeId: "sdk-folder",
      defaultExpandedIds: ["sdk-folder"],
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
];
