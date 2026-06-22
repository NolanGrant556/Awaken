import type { ComponentProps } from "react";
import type { AwakenWorkbenchChrome } from "./AwakenWorkbenchChrome";

export type AwakenWorkbenchChromeState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWorkbenchChrome>>;
};

const noop = () => {};

export const AwakenWorkbenchChromeStates: AwakenWorkbenchChromeState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      sidebarCollapsed: "AwakenWorkbenchChrome",
      onToggleSidebar: noop,
      onNewTask: noop,
      right: "AwakenWorkbenchChrome",
      children: "AwakenWorkbenchChrome",
      onClick: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenWorkbenchChrome with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
];
