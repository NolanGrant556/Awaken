import type { ComponentProps } from "react";
import type { AwakenQuickActionGrid } from "./AwakenQuickActionGrid";

export type AwakenQuickActionGridState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenQuickActionGrid>>;
};

const noop = () => {};

export const AwakenQuickActionGridStates: AwakenQuickActionGridState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenQuickActionGrid",
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionGrid>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenQuickActionGrid with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenQuickActionGrid>>,
  },
];
