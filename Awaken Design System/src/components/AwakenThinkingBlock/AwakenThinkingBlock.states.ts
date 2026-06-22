import type { ComponentProps } from "react";
import type { AwakenThinkingBlock } from "./AwakenThinkingBlock";

export type AwakenThinkingBlockState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenThinkingBlock>>;
};

export const AwakenThinkingBlockStates: AwakenThinkingBlockState[] = [
  {
    id: "active",
    label: "思考中",
    props: {
      text: "正在梳理当前工作区结构，确认需要优先阅读的组件实现和示例文件。",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenThinkingBlock>>,
  },
  {
    id: "done",
    label: "已完成思考",
    props: {
      text: "已确认这些组件只负责展示层，运行状态、排序和 Inspector 选择都由父级提供。",
      status: "done",
    } as unknown as Partial<ComponentProps<typeof AwakenThinkingBlock>>,
  },
  {
    id: "long-content",
    label: "长推理文本",
    props: {
      text:
        "正在比较执行流中的提示、思考、工具轨迹和回答之间的职责边界。\n\n这段内容用于验证 Thinking 文本较长时可以在卡片内部滚动，而不会把外层运行舞台撑开。",
      status: "active",
    } as unknown as Partial<ComponentProps<typeof AwakenThinkingBlock>>,
  },
];
