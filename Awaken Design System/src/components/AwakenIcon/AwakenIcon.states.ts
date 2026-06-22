import type { ComponentProps } from "react";
import type { AwakenIcon } from "./AwakenIcon";

export type AwakenIconState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenIcon>>;
};

const noop = () => {};

export const AwakenIconStates: AwakenIconState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      size: "md",
      strokeWidth: "AwakenIcon",
      ariaHidden: "AwakenIcon",
      children: "AwakenIcon",
    } as unknown as Partial<ComponentProps<typeof AwakenIcon>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenIcon with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenIcon>>,
  },
];
