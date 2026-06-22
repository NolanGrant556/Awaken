import type { ComponentProps } from "react";
import type { AwakenOriginalThinking } from "./AwakenOriginalThinking";

export type AwakenOriginalThinkingState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenOriginalThinking>>;
};

export const AwakenOriginalThinkingStates: AwakenOriginalThinkingState[] = [
  {
    id: "original",
    label: "Original Thinking",
    props: {},
  },
];
