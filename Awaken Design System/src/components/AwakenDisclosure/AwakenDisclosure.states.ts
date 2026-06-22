import type { ComponentProps } from "react";
import type { AwakenDisclosure } from "./AwakenDisclosure";

export type AwakenDisclosureState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenDisclosure>>;
};

const noop = () => {};

export const AwakenDisclosureStates: AwakenDisclosureState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenDisclosure",
      children: "AwakenDisclosure",
      meta: "AwakenDisclosure",
      defaultOpen: true,
      open: true,
      onOpenChange: noop,
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenDisclosure>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenDisclosure with a deliberately long value that should remain readable inside its container",
      children: "AwakenDisclosure with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenDisclosure>>,
  },
];
