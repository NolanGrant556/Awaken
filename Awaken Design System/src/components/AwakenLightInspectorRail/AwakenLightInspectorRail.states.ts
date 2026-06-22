import { createElement, type ComponentProps } from "react";
import { FileText, Folder, Globe, Terminal } from "lucide-react";
import type { AwakenLightInspectorRail } from "./AwakenLightInspectorRail";

export type AwakenLightInspectorRailState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenLightInspectorRail>>;
};

const noop = () => {};

export const AwakenLightInspectorRailStates: AwakenLightInspectorRailState[] = [
  {
    id: "changes-active",
    label: "改动选中",
    props: {
      title: "On Awaken",
      status: "reviewing",
      items: [
        {
          id: "changes",
          label: "改动",
          badge: 3,
          deltaAdded: 42,
          deltaRemoved: 8,
          tone: "success",
          icon: createElement(FileText, { size: 14, strokeWidth: 2 }),
        },
        {
          id: "terminal",
          label: "终端",
          badge: "live",
          tone: "neutral",
          icon: createElement(Terminal, { size: 14, strokeWidth: 2 }),
        },
        {
          id: "browser",
          label: "浏览器",
          disabled: true,
          icon: createElement(Globe, { size: 14, strokeWidth: 2 }),
        },
      ],
      activeId: "changes",
      onSelect: noop,
      children: "展示本次执行改动的摘要和关键文件。",
    } as unknown as Partial<ComponentProps<typeof AwakenLightInspectorRail>>,
  },
  {
    id: "terminal-warning",
    label: "终端提醒",
    props: {
      title: "运行检查器",
      status: "needs attention",
      items: [
        {
          id: "changes",
          label: "改动",
          badge: 1,
          icon: createElement(FileText, { size: 14, strokeWidth: 2 }),
        },
        {
          id: "terminal",
          label: "终端",
          badge: "失败",
          tone: "warning",
          icon: createElement(Terminal, { size: 14, strokeWidth: 2 }),
        },
      ],
      activeId: "terminal",
      children: "终端检查需要复核，父级负责提供具体输出内容。",
    } as unknown as Partial<ComponentProps<typeof AwakenLightInspectorRail>>,
  },
  {
    id: "empty-views",
    label: "暂无视图",
    props: {
      title: "检查器",
      status: "empty",
      items: [],
      activeId: undefined,
      children: "当前没有可用检查器视图。",
    } as unknown as Partial<ComponentProps<typeof AwakenLightInspectorRail>>,
  },
  {
    id: "long-label",
    label: "长导航项",
    props: {
      title: "轻量检查器",
      status: "reviewing",
      items: [
        {
          id: "workspace-files",
          label: "工作区文件树和最近打开文件",
          badge: 18,
          tone: "neutral",
          icon: createElement(Folder, { size: 14, strokeWidth: 2 }),
        },
      ],
      activeId: "workspace-files",
      children: "验证导航项标签、badge 和详情内容在轻量侧栏中保持可读。",
    } as unknown as Partial<ComponentProps<typeof AwakenLightInspectorRail>>,
  },
];
