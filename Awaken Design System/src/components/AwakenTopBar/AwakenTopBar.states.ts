import type { ComponentProps } from "react";
import type { AwakenTopBar } from "./AwakenTopBar";

export type AwakenTopBarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenTopBar>>;
};

export const AwakenTopBarStates: AwakenTopBarState[] = [
  {
    id: "workspace-status",
    label: "工作区状态",
    props: {
      children: "当前工作区：Awaken / 本地运行中",
    } as unknown as Partial<ComponentProps<typeof AwakenTopBar>>,
  },
  {
    id: "run-status",
    label: "运行状态",
    props: {
      children: "Streaming / 父级在这里放置取消、筛选或工作区操作",
    } as unknown as Partial<ComponentProps<typeof AwakenTopBar>>,
  },
  {
    id: "long-content",
    label: "较长内容",
    props: {
      children: "较长的顶部状态信息：本地 runtime 正在启动，工作区路径等待父级校验",
    } as unknown as Partial<ComponentProps<typeof AwakenTopBar>>,
  },
];
