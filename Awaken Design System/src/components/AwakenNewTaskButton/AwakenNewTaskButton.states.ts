import type { ComponentProps } from "react";
import type { AwakenNewTaskButton } from "./AwakenNewTaskButton";

export type AwakenNewTaskButtonState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenNewTaskButton>>;
};

const noop = () => {};

export const AwakenNewTaskButtonStates: AwakenNewTaskButtonState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      onClick: noop,
      shortcut: "AwakenNewTaskButton",
      platform: "macos",
      children: "AwakenNewTaskButton",
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenNewTaskButton>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenNewTaskButton with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenNewTaskButton>>,
  },
];
