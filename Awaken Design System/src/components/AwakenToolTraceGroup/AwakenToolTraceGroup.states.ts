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
    id: "mixed-active",
    label: "混合活动批次",
    props: {
      actions: [
        { id: "read-app", title: "读取 App.tsx", status: "done" },
        { id: "glob-components", title: "运行 rg --files", kind: "terminal", status: "active" },
        { id: "grep-usage", title: "搜索 ToolTrace 用法", status: "done" },
      ],
      label: "正在运行 3 个工具",
      onSelectAction: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenToolTraceGroup>>,
  },
  {
    id: "no-label",
    label: "无组标题",
    props: {
      actions: [
        { id: "read-package", title: "读取 package.json", status: "done" },
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
        { id: "read", title: "读取 AwakenToolTraceGroup.tsx", status: "done" },
        { id: "search", title: "搜索 tool-trace-group", status: "error" },
      ],
      label: "检查了 2 项",
      detail: "1 个异常",
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
