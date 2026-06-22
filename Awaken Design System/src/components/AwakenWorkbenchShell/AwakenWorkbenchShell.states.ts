import type { ComponentProps } from "react";
import type { AwakenWorkbenchShell } from "./AwakenWorkbenchShell";

export type AwakenWorkbenchShellState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWorkbenchShell>>;
};

const noop = () => {};

export const AwakenWorkbenchShellStates: AwakenWorkbenchShellState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      stage: "AwakenWorkbenchShell",
      inspector: "AwakenWorkbenchShell",
      children: "AwakenWorkbenchShell",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchShell>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenWorkbenchShell with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchShell>>,
  },
];
