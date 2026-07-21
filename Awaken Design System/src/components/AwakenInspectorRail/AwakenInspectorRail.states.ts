import type { ComponentProps } from "react";
import type { AwakenInspectorRail } from "./AwakenInspectorRail";

export type AwakenInspectorRailState = {
  id: string;
  label: string;
  props: ComponentProps<typeof AwakenInspectorRail>;
};

export const AwakenInspectorRailStates: AwakenInspectorRailState[] = [
  {
    id: "desktop-placeholder",
    label: "Desktop 当前占位",
    props: {
      title: "On Awaken",
      items: [
        {
          id: "right-sidebar-placeholder",
          label: "右侧栏待开发",
          disabled: true,
          tone: "neutral",
        },
      ],
      activeId: "right-sidebar-placeholder",
    },
  },
];
