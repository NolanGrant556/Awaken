import type { ComponentProps } from "react";
import type { AwakenProfileButton } from "./AwakenProfileButton";

export type AwakenProfileButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenProfileButton>>;
};

const noop = () => {};

export const AwakenProfileButtonStates: AwakenProfileButtonState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      name: "AwakenProfileButton",
      subtitle: "AwakenProfileButton",
      avatarFallback: "AwakenProfileButton",
      avatarSrc: "AwakenProfileButton",
      onClick: noop,
      children: "AwakenProfileButton",
    } as unknown as Partial<ComponentProps<typeof AwakenProfileButton>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenProfileButton with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenProfileButton>>,
  },
];
