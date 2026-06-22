import type { ComponentProps } from "react";
import type { AwakenWorkbenchSidebarFrame } from "./AwakenWorkbenchSidebarFrame";

export type AwakenWorkbenchSidebarFrameState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWorkbenchSidebarFrame>>;
};

const noop = () => {};

export const AwakenWorkbenchSidebarFrameStates: AwakenWorkbenchSidebarFrameState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      children: "AwakenWorkbenchSidebarFrame",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchSidebarFrame>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      children: "AwakenWorkbenchSidebarFrame with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchSidebarFrame>>,
  },
];
