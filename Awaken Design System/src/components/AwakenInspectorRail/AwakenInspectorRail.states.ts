import { createElement, type ComponentProps } from "react";
import { Folder } from "lucide-react";
import type { AwakenInspectorRail } from "./AwakenInspectorRail";

export type AwakenInspectorRailState = {
  id: string;
  label: string;
  props: ComponentProps<typeof AwakenInspectorRail>;
};

export const AwakenInspectorRailStates: AwakenInspectorRailState[] = [
  {
    id: "desktop-placeholder",
    label: "Desktop 文件入口占位",
    props: {
      ariaLabel: "文件",
      items: [
        {
          id: "files-placeholder",
          label: "文件",
          disabled: true,
          tone: "neutral",
          icon: createElement(Folder, {
            size: 16,
            strokeWidth: 1.8,
            "aria-hidden": true,
          }),
        },
      ],
      activeId: "files-placeholder",
    },
  },
];
