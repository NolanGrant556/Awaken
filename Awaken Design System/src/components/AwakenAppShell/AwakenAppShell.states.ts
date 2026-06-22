import type { ComponentProps } from "react";
import type { AwakenAppShell } from "./AwakenAppShell";

export type AwakenAppShellState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenAppShell>>;
};

const noop = () => {};

export const AwakenAppShellStates: AwakenAppShellState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      sidebar: "AwakenAppShell",
      main: "AwakenAppShell",
      chrome: "AwakenAppShell",
      sidebarCollapsed: "AwakenAppShell",
      platform: "macos",
      children: "AwakenAppShell",
    } as unknown as Partial<ComponentProps<typeof AwakenAppShell>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenAppShell with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenAppShell>>,
  },
];
