import type { ComponentProps } from "react";
import type { AwakenDropdown } from "./AwakenDropdown";

export type AwakenDropdownState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenDropdown>>;
};

const noop = () => {};

export const AwakenDropdownStates: AwakenDropdownState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      trigger: "Open",
      children: "AwakenDropdown",
      open: true,
      onOpenChange: noop,
      placement: "bottom-start",
      offset: 4,
    } as unknown as Partial<ComponentProps<typeof AwakenDropdown>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenDropdown with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenDropdown>>,
  },
];
