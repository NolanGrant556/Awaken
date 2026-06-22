import type { ComponentProps } from "react";
import type { AwakenPromptCard } from "./AwakenPromptCard";

export type AwakenPromptCardState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPromptCard>>;
};

const noop = () => {};

export const AwakenPromptCardStates: AwakenPromptCardState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      text: "AwakenPromptCard",
      eyebrow: "AwakenPromptCard",
      status: "done",
      children: "AwakenPromptCard",
    } as unknown as Partial<ComponentProps<typeof AwakenPromptCard>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      text: "AwakenPromptCard with a deliberately long value that should remain readable inside its container",
      children: "AwakenPromptCard with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenPromptCard>>,
  },
];
