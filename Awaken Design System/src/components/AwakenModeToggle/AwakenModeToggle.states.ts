import type { ComponentProps } from "react";
import type { AwakenModeToggle } from "./AwakenModeToggle";

export type AwakenModeToggleState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenModeToggle>>;
};

const noop = () => {};

export const AwakenModeToggleStates: AwakenModeToggleState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      mode: "Plan",
      onChange: noop,
      disabled: false,
      children: "AwakenModeToggle",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenModeToggle>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenModeToggle with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenModeToggle>>,
  },
];
