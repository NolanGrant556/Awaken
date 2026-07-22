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
      children: "取消",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "primary-action",
    label: "主操作",
    props: {
      variant: "primary",
      size: "md",
      children: "保存",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "ghost-action",
    label: "轻量操作",
    props: {
      variant: "ghost",
      size: "sm",
      children: "打开 Awaken 目录",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "loading",
    label: "提交中",
    props: {
      variant: "primary",
      size: "md",
      loading: true,
      children: "保存",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "disabled",
    label: "不可用",
    props: {
      variant: "default",
      size: "md",
      disabled: true,
      children: "保存",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "long-label",
    label: "长按钮文本",
    props: {
      variant: "default",
      size: "lg",
      children: "更新 API key",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
];
