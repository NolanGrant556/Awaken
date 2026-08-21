import { createElement, type ComponentProps } from "react";
import { AwakenIcon } from "../AwakenIcon";
import type { AwakenIconButton } from "./AwakenIconButton";

export type AwakenIconButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenIconButton>>;
};

export const AwakenIconButtonStates: AwakenIconButtonState[] = [
  {
    id: "add-context",
    label: "添加上下文",
    props: {
      label: "添加上下文",
      variant: "square",
      size: "sm",
      children: createElement(AwakenIcon, { concept: "add", size: 16 }),
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
  {
    id: "add-context-disabled",
    label: "上下文暂不可添加",
    props: {
      label: "添加上下文",
      variant: "square",
      size: "sm",
      children: createElement(AwakenIcon, { concept: "add", size: 16 }),
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
  {
    id: "dialog-close",
    label: "关闭弹窗",
    props: {
      label: "关闭",
      variant: "square",
      size: "sm",
      showTooltip: false,
      children: createElement(AwakenIcon, { concept: "close", size: 16 }),
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
];
