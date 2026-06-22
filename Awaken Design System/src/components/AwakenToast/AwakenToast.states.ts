import type { ComponentProps } from "react";
import type { AwakenToast } from "./AwakenToast";

export type AwakenToastState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToast>>;
};

const noop = () => {};

export const AwakenToastStates: AwakenToastState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      message: "AwakenToast",
      visible: "AwakenToast",
      duration: "AwakenToast",
      onClose: noop,
      children: "AwakenToast",
    } as unknown as Partial<ComponentProps<typeof AwakenToast>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenToast with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenToast>>,
  },
];
