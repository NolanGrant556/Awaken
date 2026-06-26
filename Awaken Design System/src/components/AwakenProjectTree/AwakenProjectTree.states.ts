import { createElement, type ComponentProps } from "react";
import { MessageCirclePlus } from "lucide-react";
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
          timeLabel: "当前",
          action: createElement(
            AwakenSidebarSectionAction,
            {
              label: "新建会话",
              children: createElement(MessageCirclePlus, { size: 13, strokeWidth: 1.85 }),
            },
          ),
          children: [
            { id: "design-system", kind: "thread", label: "Awaken Design System", timeLabel: "16" },
            { id: "desktop", kind: "thread", label: "apps/desktop", timeLabel: "8" },
          ],
        },
      ],
      activeId: "design-system",
      expandedIds: ["awaken"],
      defaultExpandedIds: ["awaken"],
      onExpandedChange: noop,
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
      expandedIds: ["workspace"],
      defaultExpandedIds: ["workspace"],
      onExpandedChange: noop,
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
          timeLabel: "今天",
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
      expandedIds: ["long-project"],
      defaultExpandedIds: ["long-project"],
      onExpandedChange: noop,
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenProjectTree>>,
  },
];
