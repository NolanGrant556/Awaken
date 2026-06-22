import type { ComponentProps } from "react";
import type { AwakenAvatar } from "./AwakenAvatar";

export type AwakenAvatarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenAvatar>>;
};

const noop = () => {};

export const AwakenAvatarStates: AwakenAvatarState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      src: "AwakenAvatar",
      fallback: "AwakenAvatar",
      size: "md",
      children: "AwakenAvatar",
    } as unknown as Partial<ComponentProps<typeof AwakenAvatar>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenAvatar with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenAvatar>>,
  },
];
