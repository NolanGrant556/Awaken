import type { ComponentProps } from "react";
import type { AwakenSendButton } from "./AwakenSendButton";

export type AwakenSendButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenSendButton>>;
};

const noop = () => {};

export const AwakenSendButtonStates: AwakenSendButtonState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      ready: "AwakenSendButton",
      onClick: noop,
      disabled: false,
      loading: false,
      mode: "send",
      children: "AwakenSendButton",
    } as unknown as Partial<ComponentProps<typeof AwakenSendButton>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenSendButton with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenSendButton>>,
  },
];
