import type { ComponentProps } from "react";
import type { AwakenQuickActionChip } from "./AwakenQuickActionChip";

export type AwakenQuickActionChipState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenQuickActionChip>>;
};

const noop = () => {};

export const AwakenQuickActionChipStates: AwakenQuickActionChipState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      icon: "AwakenQuickActionChip",
      label: "AwakenQuickActionChip",
      prompt: "AwakenQuickActionChip",
      onClick: noop,
      disabled: false,
      children: "AwakenQuickActionChip",
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionChip>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      label: "AwakenQuickActionChip with a deliberately long value that should remain readable inside its container",
      children: "AwakenQuickActionChip with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionChip>>,
  },
];
