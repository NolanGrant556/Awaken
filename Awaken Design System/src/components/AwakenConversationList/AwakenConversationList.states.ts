import type { ComponentProps } from "react";
import type { AwakenConversationList } from "./AwakenConversationList";

export type AwakenConversationListState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenConversationList>>;
};

const noop = () => {};

export const AwakenConversationListStates: AwakenConversationListState[] = [
  {
    id: "recent-runs",
    label: "最近任务",
    props: {
      items: [
        {
          id: "design-system-docs",
          label: "补全设计系统用途说明",
          timeLabel: "now",
          statusLabel: "运行中",
          statusTone: "local",
        },
        {
          id: "desktop-smoke",
          label: "桌面端冒烟检查",
          timeLabel: "18m",
          statusLabel: "已完成",
          statusTone: "success",
        },
        {
          id: "approval-flow",
          label: "审批流复核",
          timeLabel: "1h",
          statusLabel: "需确认",
          statusTone: "permission",
        },
      ],
      activeId: "design-system-docs",
      onSelect: noop,
      onContextMenu: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenConversationList>>,
  },
  {
    id: "empty",
    label: "暂无对话",
    props: {
      items: [],
      onSelect: noop,
      onContextMenu: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenConversationList>>,
  },
  {
    id: "disabled-item",
    label: "不可选任务",
    props: {
      items: [
        {
          id: "offline-runtime",
          label: "等待本地 runtime 恢复",
          timeLabel: "now",
          statusLabel: "离线",
          statusTone: "warning",
          disabled: true,
        },
        {
          id: "ready-run",
          label: "可继续的组件整理任务",
          timeLabel: "5m",
          statusLabel: "可继续",
          statusTone: "neutral",
        },
      ],
      activeId: "ready-run",
      onSelect: noop,
      onContextMenu: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenConversationList>>,
  },
  {
    id: "long-title",
    label: "长任务标题",
    props: {
      items: [
        {
          id: "long-task",
          label: "阅读所有第一组组件实现并补全 README 用途、使用场景和父子责任边界",
          timeLabel: "2h",
          statusLabel: "文档整理中",
          statusTone: "neutral",
        },
      ],
      activeId: "long-task",
      onSelect: noop,
      onContextMenu: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenConversationList>>,
  },
];
