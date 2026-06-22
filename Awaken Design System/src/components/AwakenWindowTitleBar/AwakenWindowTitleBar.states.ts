import type { ComponentProps } from "react";
import type { AwakenWindowTitleBar } from "./AwakenWindowTitleBar";

export type AwakenWindowTitleBarState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWindowTitleBar>>;
};

const noop = () => {};

export const AwakenWindowTitleBarStates: AwakenWindowTitleBarState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      platform: "macos",
      onMinimize: noop,
      onMaximize: noop,
      onClose: noop,
      children: "AwakenWindowTitleBar",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenWindowTitleBar>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenWindowTitleBar with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenWindowTitleBar>>,
  },
];
