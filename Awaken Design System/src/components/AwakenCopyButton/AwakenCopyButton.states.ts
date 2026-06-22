import type { ComponentProps } from "react";
import type { AwakenCopyButton } from "./AwakenCopyButton";

export type AwakenCopyButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenCopyButton>>;
};

const noop = () => {};

export const AwakenCopyButtonStates: AwakenCopyButtonState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      text: "AwakenCopyButton",
      label: "AwakenCopyButton",
      copiedLabel: "AwakenCopyButton",
      errorLabel: "AwakenCopyButton",
      onCopied: noop,
      onCopyError: noop,
      children: "AwakenCopyButton",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenCopyButton>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      text: "AwakenCopyButton with a deliberately long value that should remain readable inside its container",
      label: "AwakenCopyButton with a deliberately long value that should remain readable inside its container",
      children: "AwakenCopyButton with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenCopyButton>>,
  },
];
