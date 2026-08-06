import { createElement, type ComponentProps } from "react";
import { Folder, Globe2, Image as ImageIcon } from "lucide-react";
import type { AwakenInspectorRail } from "./AwakenInspectorRail";
import { AwakenMediaInspector } from "../AwakenMediaInspector";

const fileAndBrowserItems = [
  {
    id: "files",
    label: "文件",
    tone: "neutral" as const,
    icon: createElement(Folder, {
      size: 16,
      strokeWidth: 1.8,
      "aria-hidden": true,
    }),
  },
  {
    id: "browser",
    label: "浏览器",
    tone: "neutral" as const,
    icon: createElement(Globe2, {
      size: 16,
      strokeWidth: 1.8,
      "aria-hidden": true,
    }),
  },
];

export type AwakenInspectorRailState = {
  id: string;
  label: string;
  props: ComponentProps<typeof AwakenInspectorRail>;
};

export const AwakenInspectorRailStates: AwakenInspectorRailState[] = [
  {
    id: "desktop-default-entries",
    label: "Desktop 默认入口",
    props: {
      ariaLabel: "文件和浏览器",
      items: fileAndBrowserItems,
    },
  },
  {
    id: "desktop-pinned-media",
    label: "用户固定媒体",
    props: {
      ariaLabel: "媒体",
      activeId: "media",
      items: [
        ...fileAndBrowserItems,
        {
          id: "media",
          label: "媒体",
          tone: "neutral",
          icon: createElement(ImageIcon, {
            size: 16,
            strokeWidth: 1.8,
            "aria-hidden": true,
          }),
        },
      ],
      children: createElement(AwakenMediaInspector, {
        item: {
          id: "gallery-media",
          kind: "image",
          title: "固定图片",
          metadata: { width: 1792, height: 2240 },
          formatLabel: "PNG",
        },
        sourceUnavailable: true,
        onClose: () => undefined,
      }),
    },
  },
];
