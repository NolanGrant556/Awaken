import type { ComponentProps } from "react";
import type { AwakenStatusPill } from "./AwakenStatusPill";

export type AwakenStatusPillState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenStatusPill>>;
};

const noop = () => {};

export const AwakenStatusPillStates: AwakenStatusPillState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      tone: "neutral",
      icon: "AwakenStatusPill",
      children: "AwakenStatusPill",
    } as unknown as Partial<ComponentProps<typeof AwakenStatusPill>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenStatusPill with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenStatusPill>>,
  },
];
