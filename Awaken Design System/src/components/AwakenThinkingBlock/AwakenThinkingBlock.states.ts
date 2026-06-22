import type { ComponentProps } from "react";
import type { AwakenThinkingBlock } from "./AwakenThinkingBlock";

export type AwakenThinkingBlockState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenThinkingBlock>>;
};

const noop = () => {};

export const AwakenThinkingBlockStates: AwakenThinkingBlockState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      text: "AwakenThinkingBlock",
      status: "done",
      children: "AwakenThinkingBlock",
    } as unknown as Partial<ComponentProps<typeof AwakenThinkingBlock>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      text: "AwakenThinkingBlock with a deliberately long value that should remain readable inside its container",
      children: "AwakenThinkingBlock with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenThinkingBlock>>,
  },
];
