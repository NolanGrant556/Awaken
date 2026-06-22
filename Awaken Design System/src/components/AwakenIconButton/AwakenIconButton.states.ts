import type { ComponentProps } from "react";
import type { AwakenIconButton } from "./AwakenIconButton";

export type AwakenIconButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenIconButton>>;
};

const noop = () => {};

export const AwakenIconButtonStates: AwakenIconButtonState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      label: "AwakenIconButton",
      variant: "circle",
      size: "md",
      children: "AwakenIconButton",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      label: "AwakenIconButton with a deliberately long value that should remain readable inside its container",
      children: "AwakenIconButton with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenIconButton>>,
  },
];
