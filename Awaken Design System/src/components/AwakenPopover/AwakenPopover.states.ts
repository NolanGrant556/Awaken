import type { ComponentProps } from "react";
import type { AwakenPopover } from "./AwakenPopover";

export type AwakenPopoverState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPopover>>;
};

const noop = () => {};

export const AwakenPopoverStates: AwakenPopoverState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      trigger: "Open",
      content: "Popover content",
      open: true,
      onOpenChange: noop,
      placement: "bottom-start",
      offset: 4,
      matchTriggerWidth: false,
      children: "AwakenPopover",
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenPopover with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenPopover>>,
  },
];
