import type { ComponentProps } from "react";
import type { AwakenToolActionRow } from "./AwakenToolActionRow";

export type AwakenToolActionRowState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolActionRow>>;
};

const noop = () => {};

export const AwakenToolActionRowStates: AwakenToolActionRowState[] = [
  {
    id: "active",
    label: "执行中",
    props: {
      title: "读取 package.json",
      detail: "确认 Design System 的脚本和依赖。",
      status: "active",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolActionRow>>,
  },
  {
    id: "done",
    label: "已完成",
    props: {
      title: "完成组件状态核对",
      detail: "已确认 demo、states 和 README 的展示入口。",
      status: "done",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolActionRow>>,
  },
  {
    id: "error",
    label: "执行失败",
    props: {
      title: "运行类型检查失败",
      detail: "需要打开终端输出查看失败原因。",
      status: "error",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolActionRow>>,
  },
  {
    id: "static",
    label: "仅展示",
    props: {
      title: "记录工具动作",
      detail: "没有传入选择回调时，这一行只呈现状态。",
      status: "done",
      onSelect: undefined,
    } as unknown as Partial<ComponentProps<typeof AwakenToolActionRow>>,
  },
  {
    id: "long-content",
    label: "长动作",
    props: {
      title: "正在检查一条标题很长的工具动作，确认它在窄容器里仍然保持可读",
      detail: "用于验证工具动作摘要的标题和细节都能正确换行。",
      status: "active",
      onSelect: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolActionRow>>,
  },
];
