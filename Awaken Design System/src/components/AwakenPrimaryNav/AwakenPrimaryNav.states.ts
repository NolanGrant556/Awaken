import type { ComponentProps } from "react";
import type { AwakenPrimaryNav } from "./AwakenPrimaryNav";

export type AwakenPrimaryNavState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenPrimaryNav>>;
};

const noop = () => {};

export const AwakenPrimaryNavStates: AwakenPrimaryNavState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      items: [{ id: "changes", label: "Changes", badge: 1 }],
      activeId: "default",
      onSelect: noop,
      children: "AwakenPrimaryNav",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenPrimaryNav>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenPrimaryNav with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenPrimaryNav>>,
  },
];
