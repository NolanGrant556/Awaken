import type { ComponentProps } from "react";
import type { AwakenRunTimeline } from "./AwakenRunTimeline";

export type AwakenRunTimelineState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenRunTimeline>>;
};

export const AwakenRunTimelineStates: AwakenRunTimelineState[] = [
  {
    id: "running",
    label: "运行中",
    props: {
      title: "补齐第 3 组组件用途说明",
      status: "running",
      steps: [
        {
          id: "scope",
          index: 1,
          title: "确认文件边界",
          description: "只改本组 demo、states、README。",
          tag: "scope",
          status: "done",
        },
        {
          id: "implementation",
          index: 2,
          title: "读取组件实现",
          description: "从 props 和 JSX 判断真实用途。",
          tag: "tsx",
          status: "active",
        },
        {
          id: "states",
          index: 3,
          title: "替换状态示例",
          description: "等待写入真实中文场景。",
          tag: "states",
          status: "pending",
        },
      ],
    } as unknown as Partial<ComponentProps<typeof AwakenRunTimeline>>,
  },
  {
    id: "ready-empty",
    label: "就绪无步骤",
    props: {
      title: "等待用户提交任务",
      status: "ready",
      steps: [],
      children: null,
    } as unknown as Partial<ComponentProps<typeof AwakenRunTimeline>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      title: "组件文档已更新",
      status: "done",
      steps: [
        {
          id: "demo",
          index: 1,
          title: "更新 demo header",
          description: "每个组件都有具体用途句。",
          tag: "demo",
          status: "done",
        },
        {
          id: "readme",
          index: 2,
          title: "补充 README 用途",
          description: "说明使用场景和父子责任边界。",
          tag: "docs",
          status: "done",
        },
      ],
      children: "父级可以在时间线底部附加验收摘要或下一步提示。",
    } as unknown as Partial<ComponentProps<typeof AwakenRunTimeline>>,
  },
  {
    id: "error",
    label: "出错",
    props: {
      title: "状态示例校验",
      status: "error",
      steps: [
        {
          id: "invalid-props",
          index: 1,
          title: "发现无效 props",
          description: "示例里仍存在无效 props 或含糊示例。",
          tag: "props",
          status: "error",
        },
      ],
      children: null,
    } as unknown as Partial<ComponentProps<typeof AwakenRunTimeline>>,
  },
  {
    id: "long-title",
    label: "长标题",
    props: {
      title: "围绕 Awaken 桌面端运行工作区整理设计系统第 3 组组件的用途、状态和责任边界",
      status: "running",
      steps: [
        {
          id: "long-step",
          index: 1,
          title: "检查一条很长的运行步骤标题是否在时间线标题区域保持截断和可读",
          description: "用于覆盖长文本状态。",
          tag: "design-system",
          status: "active",
        },
      ],
      children: null,
    } as unknown as Partial<ComponentProps<typeof AwakenRunTimeline>>,
  },
];
