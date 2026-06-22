import type { ComponentProps } from "react";
import type { AwakenContextButton } from "./AwakenContextButton";

export type AwakenContextButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenContextButton>>;
};

const noop = () => {};

export const AwakenContextButtonStates: AwakenContextButtonState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      onClick: noop,
      disabled: false,
      children: "AwakenContextButton",
    } as unknown as Partial<ComponentProps<typeof AwakenContextButton>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenContextButton with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenContextButton>>,
  },
];
