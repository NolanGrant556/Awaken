import type { ComponentProps } from "react";
import type { AwakenToast } from "./AwakenToast";

export type AwakenToastState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToast>>;
};

const noop = () => {};

export const AwakenToastStates: AwakenToastState[] = [
  {
    id: "visible",
    label: "可见提示",
    props: {
      message: "工作区已切换到 /Users/nolangrant/Awaken",
      visible: true,
      duration: 5000,
      onClose: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToast>>,
  },
  {
    id: "hidden",
    label: "隐藏状态",
    props: {
      message: "这条提示由父级隐藏，不会渲染到画布。",
      visible: false,
      duration: 1500,
      onClose: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToast>>,
  },
  {
    id: "long-content",
    label: "长提示",
    props: {
      message: "运行启动失败：请确认本地 runtime 已启动，并检查工作区路径是否为绝对路径。",
      visible: true,
      duration: 5000,
      onClose: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToast>>,
  },
];
