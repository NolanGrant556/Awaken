import type { ComponentProps } from "react";
import type { AwakenDivider } from "./AwakenDivider";

export type AwakenDividerState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenDivider>>;
};

const noop = () => {};

export const AwakenDividerStates: AwakenDividerState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      spacing: "AwakenDivider",
      children: "AwakenDivider",
    } as unknown as Partial<ComponentProps<typeof AwakenDivider>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenDivider with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenDivider>>,
  },
];
