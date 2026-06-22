import type { ComponentProps } from "react";
import type { AwakenUserPromptBlock } from "./AwakenUserPromptBlock";

export type AwakenUserPromptBlockState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenUserPromptBlock>>;
};

const noop = () => {};

export const AwakenUserPromptBlockStates: AwakenUserPromptBlockState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      text: "AwakenUserPromptBlock",
      children: "AwakenUserPromptBlock",
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      text: "AwakenUserPromptBlock with a deliberately long value that should remain readable inside its container",
      children: "AwakenUserPromptBlock with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenUserPromptBlock>>,
  },
];
