import type { ComponentProps } from "react";
import type { AwakenStatusPill } from "./AwakenStatusPill";

export type AwakenStatusPillState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenStatusPill>>;
};

export const AwakenStatusPillStates: AwakenStatusPillState[] = [
  {
    id: "local",
    label: "本地运行",
    props: {
      tone: "local",
      icon: "●",
      children: "本地运行中",
    } as unknown as Partial<ComponentProps<typeof AwakenStatusPill>>,
  },
  {
    id: "permission",
    label: "需要确认",
    props: {
      tone: "permission",
      icon: "!",
      children: "等待权限确认",
    } as unknown as Partial<ComponentProps<typeof AwakenStatusPill>>,
  },
  {
    id: "success",
    label: "已完成",
    props: {
      tone: "success",
      icon: "✓",
      children: "运行已完成",
    } as unknown as Partial<ComponentProps<typeof AwakenStatusPill>>,
  },
  {
    id: "error",
    label: "错误",
    props: {
      tone: "error",
      icon: "!",
      children: "运行出错",
    } as unknown as Partial<ComponentProps<typeof AwakenStatusPill>>,
  },
  {
    id: "long-content",
    label: "较长状态",
    props: {
      tone: "warning",
      children: "文件变更已准备好等待复核",
    } as unknown as Partial<ComponentProps<typeof AwakenStatusPill>>,
  },
];
