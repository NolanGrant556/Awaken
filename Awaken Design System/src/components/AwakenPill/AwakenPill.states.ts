import type { ComponentProps } from "react";
import type { AwakenPill } from "./AwakenPill";

export type AwakenPillState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPill>>;
};

const noop = () => {};

export const AwakenPillStates: AwakenPillState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      variant: "default",
      size: "md",
      icon: "AwakenPill",
      active: false,
      children: "AwakenPill",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPill>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenPill with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenPill>>,
  },
];
