import type { ComponentProps } from "react";
import type { AwakenTopBar } from "./AwakenTopBar";

export type AwakenTopBarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenTopBar>>;
};

const noop = () => {};

export const AwakenTopBarStates: AwakenTopBarState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenTopBar",
    } as unknown as Partial<ComponentProps<typeof AwakenTopBar>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenTopBar with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenTopBar>>,
  },
];
