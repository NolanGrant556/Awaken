import type { ComponentProps } from "react";
import type { AwakenExploringGroup } from "./AwakenExploringGroup";

export type AwakenExploringGroupState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenExploringGroup>>;
};

const noop = () => {};

export const AwakenExploringGroupStates: AwakenExploringGroupState[] = [
  {
    id: "active-exploration",
    label: "探索中",
    props: {
      title: "正在探索上下文",
      status: "active",
      actions: [
        { id: "read-component", title: "读取组件实现", detail: "确认 props 和渲染边界", status: "done" },
        { id: "scan-docs", title: "扫描 README", detail: "查找是否已有用途说明", status: "active" },
        { id: "check-states", title: "检查状态示例", detail: "补齐真实语义状态", status: "active" },
      ],
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenExploringGroup>>,
  },
  {
    id: "done-exploration",
    label: "探索完成",
    props: {
      title: "上下文收集完成",
      status: "done",
      actions: [
        { id: "read-props", title: "已读取 props", detail: "用途说明基于真实组件 API", status: "done" },
        { id: "draft-copy", title: "已整理中文说明", detail: "准备写入 demo、states 和 README", status: "done" },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenExploringGroup>>,
  },
  {
    id: "empty-actions",
    label: "暂无动作",
    props: {
      title: "等待探索动作",
      status: "active",
      actions: [],
    } as unknown as Partial<ComponentProps<typeof AwakenExploringGroup>>,
  },
  {
    id: "long-action",
    label: "长动作说明",
    props: {
      title: "正在复核跨目录组件用途，确保每一条动作在窄容器内仍然能换行展示",
      status: "active",
      actions: [
        {
          id: "long-detail",
          title: "读取 Awaken Design System 中较长路径下的组件状态定义",
          detail: "用于验证 ToolActionRow 子项承载长标题和长说明时，ExploringGroup 只负责列表归组。",
          status: "active",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenExploringGroup>>,
  },
];
