import type { ComponentProps } from "react";
import type { AwakenModelSelector } from "./AwakenModelSelector";

export type AwakenModelSelectorState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenModelSelector>>;
};

const noop = () => {};

export const AwakenModelSelectorStates: AwakenModelSelectorState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      value: { id: "default", label: "Default" },
      options: [{ id: "default", label: "Default" }],
      onChange: noop,
      disabled: false,
      children: "AwakenModelSelector",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenModelSelector>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      value: { id: "default", label: "Default" },
      children: "AwakenModelSelector with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenModelSelector>>,
  },
];
