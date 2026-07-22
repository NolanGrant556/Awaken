import type { ComponentProps } from "react";
import type { AwakenTooltip } from "./AwakenTooltip";

export type AwakenTooltipState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenTooltip>>;
};

export const AwakenTooltipStates: AwakenTooltipState[] = [
  {
    id: "add-folder",
    label: "侧栏添加文件夹",
    props: {
      label: "Add folder",
      children: "添加文件夹按钮",
      placement: "bottom-start",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
  {
    id: "send",
    label: "输入栏发送",
    props: {
      label: "发送",
      children: "发送按钮",
      placement: "top",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
  {
    id: "stop",
    label: "输入栏停止运行",
    props: {
      label: "停止运行",
      children: "停止按钮",
      placement: "top",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
  {
    id: "copy-answer",
    label: "复制回答",
    props: {
      label: "Copy final answer",
      children: "复制按钮",
      placement: "top",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
];
