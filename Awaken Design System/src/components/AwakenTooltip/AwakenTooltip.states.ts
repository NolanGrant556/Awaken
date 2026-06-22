import type { ComponentProps } from "react";
import type { AwakenTooltip } from "./AwakenTooltip";

export type AwakenTooltipState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenTooltip>>;
};

const noop = () => {};

export const AwakenTooltipStates: AwakenTooltipState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      label: "AwakenTooltip",
      children: "AwakenTooltip",
      placement: "bottom-start",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      label: "AwakenTooltip with a deliberately long value that should remain readable inside its container",
      children: "AwakenTooltip with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenTooltip>>,
  },
];
