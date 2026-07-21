import { createElement, type ComponentProps } from "react";
import { AwakenInspectorRail } from "../AwakenInspectorRail";
import type { AwakenWorkbenchShell } from "./AwakenWorkbenchShell";

export type AwakenWorkbenchShellState = {
  id: string;
  label: string;
  description: string;
  props: ComponentProps<typeof AwakenWorkbenchShell>;
};

const currentInspector = createElement(AwakenInspectorRail, {
  title: "On Awaken",
  items: [
    {
      id: "right-sidebar-placeholder",
      label: "右侧栏待开发",
      disabled: true,
    },
  ],
  activeId: "right-sidebar-placeholder",
});

export const AwakenWorkbenchShellStates: AwakenWorkbenchShellState[] = [
  {
    id: "desktop-current",
    label: "主舞台 + 完整右侧栏",
    description: "当前 Workbench 的正常结构：运行舞台使用剩余空间，Inspector 保持独立右栏宽度。",
    props: {
      stage: "运行舞台",
      inspector: currentInspector,
    },
  },
  {
    id: "inspector-collapsed",
    label: "右侧栏已折叠",
    description: "折叠后 Inspector 不再占用宽度，运行舞台扩展到整行；开关仍由顶部控制栏提供。",
    props: {
      stage: "运行舞台扩展到全部可用宽度",
      inspector: currentInspector,
      inspectorCollapsed: true,
    },
  },
  {
    id: "stage-only",
    label: "组件能力：只显示主舞台",
    description: "未传入 Inspector 时，这个布局组件只保留主舞台，不会生成空白右栏。",
    props: {
      stage: "只显示主舞台",
    },
  },
];
