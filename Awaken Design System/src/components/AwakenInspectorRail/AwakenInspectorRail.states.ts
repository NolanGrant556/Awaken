import type { ComponentProps } from "react";
import type { AwakenInspectorRail } from "./AwakenInspectorRail";

export type AwakenInspectorRailState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenInspectorRail>>;
};

const noop = () => {};

export const AwakenInspectorRailStates: AwakenInspectorRailState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenInspectorRail",
      status: "done",
      tabs: "Changes / Terminal",
      children: "AwakenInspectorRail",
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorRail>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenInspectorRail with a deliberately long value that should remain readable inside its container",
      children: "AwakenInspectorRail with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorRail>>,
  },
];
