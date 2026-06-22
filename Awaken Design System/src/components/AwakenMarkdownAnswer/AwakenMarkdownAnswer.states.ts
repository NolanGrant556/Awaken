import type { ComponentProps } from "react";
import type { AwakenMarkdownAnswer } from "./AwakenMarkdownAnswer";

export type AwakenMarkdownAnswerState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenMarkdownAnswer>>;
};

const noop = () => {};

export const AwakenMarkdownAnswerStates: AwakenMarkdownAnswerState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      markdown: "AwakenMarkdownAnswer **markdown**",
      status: "done",
      children: "AwakenMarkdownAnswer",
    } as unknown as Partial<ComponentProps<typeof AwakenMarkdownAnswer>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      markdown: "AwakenMarkdownAnswer with a deliberately long value that should remain readable inside its container **markdown**",
      children: "AwakenMarkdownAnswer with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenMarkdownAnswer>>,
  },
];
