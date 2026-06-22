import type { ComponentProps } from "react";
import type { AwakenTooltip } from "./AwakenTooltip";

export type AwakenTooltipState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenTooltip>>;
};

export const AwakenTooltipStates: AwakenTooltipState[] = [
  {
    id: "bottom-start",
    label: "按钮说明",
    props: {
      label: "切换侧栏",
      children: "侧栏按钮",
      placement: "bottom-start",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
  {
    id: "bottom",
    label: "底部提示",
    props: {
      label: "选择当前运行使用的工作区",
      children: "选择工作区",
      placement: "bottom",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
  {
    id: "disabled",
    label: "禁用提示",
    props: {
      label: "运行忙碌时不显示 Tooltip",
      children: "正在运行",
      placement: "top",
      disabled: true,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
  {
    id: "long-content",
    label: "长说明",
    props: {
      label: "用于解释一个紧凑图标按钮的具体作用，避免用户只能从图标猜测行为。",
      children: "权限说明",
      placement: "top",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
];
