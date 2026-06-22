import type { ComponentProps } from "react";
import type { AwakenThoughtTrace } from "./AwakenThoughtTrace";

export type AwakenThoughtTraceState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenThoughtTrace>>;
};

const noop = () => {};

export const AwakenThoughtTraceStates: AwakenThoughtTraceState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      text: "AwakenThoughtTrace",
      status: "done",
      label: "AwakenThoughtTrace",
      durationLabel: "AwakenThoughtTrace",
      defaultOpen: true,
      children: "AwakenThoughtTrace",
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      text: "AwakenThoughtTrace with a deliberately long value that should remain readable inside its container",
      label: "AwakenThoughtTrace with a deliberately long value that should remain readable inside its container",
      children: "AwakenThoughtTrace with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenThoughtTrace>>,
  },
];
