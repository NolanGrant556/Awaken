import type { ComponentProps } from "react";
import type { AwakenPill } from "./AwakenPill";

export type AwakenPillState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPill>>;
};

export const AwakenPillStates: AwakenPillState[] = [
  {
    id: "filter-idle",
    label: "筛选项",
    props: {
      variant: "default",
      size: "md",
      icon: "任务",
      active: false,
      children: "全部任务",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPill>>,
  },
  {
    id: "active-mode",
    label: "已选模式",
    props: {
      variant: "default",
      size: "md",
      icon: "模式",
      active: true,
      children: "Act",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPill>>,
  },
  {
    id: "bordered-small",
    label: "小号边框",
    props: {
      variant: "bordered",
      size: "sm",
      icon: "本地",
      active: false,
      children: "本地上下文",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPill>>,
  },
  {
    id: "disabled",
    label: "不可选择",
    props: {
      variant: "bordered",
      size: "md",
      icon: "云端",
      active: false,
      children: "远程执行",
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenPill>>,
  },
  {
    id: "long-label",
    label: "长标签",
    props: {
      variant: "default",
      size: "md",
      icon: "筛选",
      active: true,
      children: "只看包含本机权限请求的运行记录",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPill>>,
  },
];
