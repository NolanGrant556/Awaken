import type { ComponentProps } from "react";
import type { AwakenToolTraceGroup } from "./AwakenToolTraceGroup";

export type AwakenToolTraceGroupState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolTraceGroup>>;
};

const noop = () => {};

export const AwakenToolTraceGroupStates: AwakenToolTraceGroupState[] = [
  {
    id: "exploring",
    label: "探索工具组",
    props: {
      actions: [
        { id: "read-app", title: "读取 App.tsx", detail: "确认执行流真实引用。", status: "done" },
        { id: "read-component", title: "阅读组件实现", detail: "提取 props 和渲染边界。", status: "active" },
        { id: "check-demo", title: "核对 Gallery 状态", detail: "等待 states 文案更新。", status: "done" },
      ],
      label: "探索项目结构",
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "no-label",
    label: "无组标题",
    props: {
      actions: [
        { id: "typecheck", title: "运行类型检查", detail: "父级可点击后打开终端详情。", status: "done" },
      ],
      label: undefined,
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "error",
    label: "包含错误",
    props: {
      actions: [
        { id: "read", title: "读取组件 props", detail: "已完成。", status: "done" },
        { id: "build", title: "打开 Gallery 预览", detail: "本地端口不可用。", status: "error" },
      ],
      label: "验证组件展示",
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "long-content",
    label: "长轨迹组",
    props: {
      actions: [
        {
          id: "long-action",
          title: "正在检查一组较长的工具轨迹标题，确认分组内每一行都能保持清晰",
          detail: "适合模拟多条工具调用被聚合在同一个探索阶段下的展示效果。",
          status: "active",
        },
      ],
      label: "较长的探索阶段标题，用于验证组标题在窄容器中不会遮挡内容",
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
];
