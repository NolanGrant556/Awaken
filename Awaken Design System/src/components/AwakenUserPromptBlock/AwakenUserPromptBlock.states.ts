import type { ComponentProps } from "react";
import type { AwakenUserPromptBlock } from "./AwakenUserPromptBlock";

export type AwakenUserPromptBlockState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenUserPromptBlock>>;
};

export const AwakenUserPromptBlockStates: AwakenUserPromptBlockState[] = [
  {
    id: "prompt",
    label: "用户提示",
    props: {
      text: "请检查 Awaken Design System 里这一组组件的用途说明是否准确。",
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "follow-up",
    label: "追问提示",
    props: {
      text: "继续上一轮修改，再检查 states 里是否还有临时示例文案。",
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "long-content",
    label: "长提示",
    props: {
      text:
        "请基于真实组件实现写清楚用途、使用场景和父子责任边界，不要只根据组件名猜测，也不要改组件实现文件或 Gallery 框架。",
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
];
