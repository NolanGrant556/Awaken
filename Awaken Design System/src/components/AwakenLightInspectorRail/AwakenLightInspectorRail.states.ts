import type { ComponentProps } from "react";
import type { AwakenLightInspectorRail } from "./AwakenLightInspectorRail";

export type AwakenLightInspectorRailState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenLightInspectorRail>>;
};

const noop = () => {};

export const AwakenLightInspectorRailStates: AwakenLightInspectorRailState[] = [
  {
    id: "default",
    label: "Default",
    props: {
      title: "AwakenLightInspectorRail",
      status: "done",
      items: [{ id: "changes", label: "Changes", badge: 1 }],
      activeId: "default",
      onSelect: noop,
      children: "AwakenLightInspectorRail",
      onClick: noop,
      disabled: false,
    } as unknown as Partial<ComponentProps<typeof AwakenLightInspectorRail>>,
  },
  {
    id: "long-content",
    label: "Long content",
    props: {
      title: "AwakenLightInspectorRail with a deliberately long value that should remain readable inside its container",
      children: "AwakenLightInspectorRail with a deliberately long value that should remain readable inside its container",
    } as unknown as Partial<ComponentProps<typeof AwakenLightInspectorRail>>,
  },
];
