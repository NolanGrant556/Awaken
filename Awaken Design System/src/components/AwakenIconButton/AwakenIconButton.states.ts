import { createElement, type ComponentProps } from "react";
import { PanelRightClose, RefreshCw, X } from "lucide-react";
import type { AwakenIconButton } from "./AwakenIconButton";

export type AwakenIconButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenIconButton>>;
};

export const AwakenIconButtonStates: AwakenIconButtonState[] = [
  {
    id: "close-inspector",
    label: "关闭检查器",
    props: {
      label: "关闭检查器",
      variant: "square",
      size: "md",
      children: createElement(PanelRightClose, { size: 16, strokeWidth: 2 }),
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
  {
    id: "refresh-small",
    label: "小圆形按钮",
    props: {
      label: "重新运行检查",
      variant: "circle",
      size: "sm",
      children: createElement(RefreshCw, { size: 15, strokeWidth: 2 }),
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
  {
    id: "disabled",
    label: "禁用状态",
    props: {
      label: "当前无法关闭",
      variant: "square",
      size: "md",
      children: createElement(X, { size: 16, strokeWidth: 2 }),
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
  {
    id: "long-label",
    label: "长提示文案",
    props: {
      label: "关闭右侧检查器并回到主执行叙事视图",
      variant: "square",
      size: "md",
      children: createElement(PanelRightClose, { size: 16, strokeWidth: 2 }),
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
  {
    id: "accessible-label-only",
    label: "仅保留无障碍名称",
    props: {
      label: "关闭",
      variant: "square",
      size: "sm",
      showTooltip: false,
      children: createElement(X, { size: 15, strokeWidth: 1.9 }),
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
];
