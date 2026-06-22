import type { ComponentProps } from "react";
import type { AwakenToolTraceLine } from "./AwakenToolTraceLine";

export type AwakenToolTraceLineState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolTraceLine>>;
};

const noop = () => {};

export const AwakenToolTraceLineStates: AwakenToolTraceLineState[] = [
  {
    id: "active",
    label: "工具执行中",
    props: {
      title: "运行 npm run typecheck",
      detail: "等待 TypeScript 输出。",
      status: "active",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      title: "读取组件实现",
      detail: "AwakenToolTraceLine.tsx 已确认。",
      status: "done",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "error",
    label: "工具失败",
    props: {
      title: "打开 Gallery 预览失败",
      detail: "端口不可用，需要父级展示终端详情。",
      status: "error",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "static",
    label: "仅展示",
    props: {
      title: "记录补充信息",
      detail: "没有选择回调时，这一行不会表现为按钮。",
      status: "done",
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
  {
    id: "long-content",
    label: "长轨迹",
    props: {
      title: "正在处理一个很长的工具调用标题，确认它在执行流里仍然能换行显示",
      detail: "细节文本可以包含文件路径、命令摘要或运行结果，由父级决定是否可点击进入 Inspector。",
      status: "active",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceLine>>,
  },
];
