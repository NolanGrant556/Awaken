import type { ComponentProps } from "react";
import type { AwakenToolTraceGroup } from "./AwakenToolTraceGroup";

export type AwakenToolTraceGroupState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenToolTraceGroup>>;
};

const noop = () => {};

export const AwakenToolTraceGroupStates: AwakenToolTraceGroupState[] = [
  {
    id: "readonly-tools",
    label: "只读探索工具",
    props: {
      actions: [
        { id: "read-app", title: "Read apps/desktop/src/renderer/App.tsx", detail: "Tool returned content", status: "done" },
        { id: "glob-components", title: "List **/AwakenToolTrace*.tsx", detail: "Tool invocation started", status: "active" },
        { id: "grep-usage", title: "Search \"ToolTrace\" in apps/desktop", detail: "Tool returned matches", status: "done" },
      ],
      label: undefined,
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "no-label",
    label: "无组标题",
    props: {
      actions: [
        { id: "read-package", title: "Read package.json", detail: "Tool returned content", status: "done" },
      ],
      label: undefined,
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "error",
    label: "包含错误",
    props: {
      actions: [
        { id: "read", title: "Read AwakenToolTraceGroup.tsx", detail: "Tool returned content", status: "done" },
        { id: "search", title: "Search \"tool-trace-group\"", detail: "Tool returned an error", status: "error" },
      ],
      label: undefined,
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "long-content",
    label: "长轨迹组",
    props: {
      actions: [
        {
          id: "long-action",
          title: "Search \"AwakenToolTraceGroup + AwakenToolTraceLine\" in Awaken Design System/src/components",
          detail: "Tool is running",
          status: "active",
        },
      ],
      label: undefined,
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
];
