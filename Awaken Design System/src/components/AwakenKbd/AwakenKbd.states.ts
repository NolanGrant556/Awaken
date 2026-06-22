import type { ComponentProps } from "react";
import type { AwakenKbd } from "./AwakenKbd";

export type AwakenKbdState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenKbd>>;
};

const noop = () => {};

export const AwakenKbdStates: AwakenKbdState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenKbd",
      platform: "macos",
    } as unknown as Partial<ComponentProps<typeof AwakenKbd>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenKbd with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenKbd>>,
  },
];
