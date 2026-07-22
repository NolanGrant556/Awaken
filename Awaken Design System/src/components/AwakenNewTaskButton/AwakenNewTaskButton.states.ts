import type { ComponentProps } from "react";
import type { AwakenNewTaskButton } from "./AwakenNewTaskButton";

export type AwakenNewTaskButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenNewTaskButton>>;
};

const noop = () => {};

export const AwakenNewTaskButtonStates: AwakenNewTaskButtonState[] = [
  {
    id: "sidebar-entry",
    label: "侧边栏入口",
    props: {
      onClick: noop,
      children: "新建任务",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenNewTaskButton>>,
  },
  {
    id: "disabled",
    label: "暂不可用",
    props: {
      onClick: noop,
      children: "正在启动运行时",
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenNewTaskButton>>,
  },
  {
    id: "long-label",
    label: "长入口文案",
    props: {
      onClick: noop,
      children: "新建带当前工作区上下文的任务",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenNewTaskButton>>,
  },
];
