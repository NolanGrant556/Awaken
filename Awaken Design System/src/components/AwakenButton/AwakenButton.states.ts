import type { ComponentProps } from "react";
import type { AwakenButton } from "./AwakenButton";

export type AwakenButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenButton>>;
};

export const AwakenButtonStates: AwakenButtonState[] = [
  {
    id: "secondary-action",
    label: "次要操作",
    props: {
      variant: "default",
      size: "md",
      loading: false,
      children: "查看详情",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "primary-action",
    label: "主操作",
    props: {
      variant: "primary",
      size: "md",
      children: "开始运行",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "ghost-action",
    label: "轻量操作",
    props: {
      variant: "ghost",
      size: "sm",
      children: "稍后",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "loading",
    label: "提交中",
    props: {
      variant: "primary",
      size: "md",
      loading: true,
      children: "正在保存",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "disabled",
    label: "不可用",
    props: {
      variant: "default",
      size: "md",
      disabled: true,
      children: "等待输入",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "long-label",
    label: "长按钮文本",
    props: {
      variant: "default",
      size: "lg",
      children: "打开本次运行的完整审计记录",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
];
