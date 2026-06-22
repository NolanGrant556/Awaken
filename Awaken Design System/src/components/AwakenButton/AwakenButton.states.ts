import type { ComponentProps } from "react";
import type { AwakenButton } from "./AwakenButton";

export type AwakenButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenButton>>;
};

const noop = () => {};

export const AwakenButtonStates: AwakenButtonState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      variant: "default",
      size: "md",
      icon: "AwakenButton",
      loading: false,
      children: "AwakenButton",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenButton with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenButton>>,
  },
];
