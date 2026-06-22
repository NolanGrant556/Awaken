import type { ComponentProps } from "react";
import type { AwakenInspectorTabs } from "./AwakenInspectorTabs";

export type AwakenInspectorTabsState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenInspectorTabs>>;
};

const noop = () => {};

export const AwakenInspectorTabsStates: AwakenInspectorTabsState[] = [
  {
    id: "changes-active",
    label: "改动选中",
    props: {
      tabs: [
        { id: "changes", label: "改动", badge: 3 },
        { id: "browser", label: "浏览器" },
        { id: "terminal", label: "终端", badge: "live" },
        { id: "files", label: "文件" },
      ],
      activeTabId: "changes",
      onTabChange: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorTabs>>,
  },
  {
    id: "terminal-active",
    label: "终端选中",
    props: {
      tabs: [
        { id: "changes", label: "改动", badge: 1 },
        { id: "browser", label: "浏览器" },
        { id: "terminal", label: "终端", badge: "运行中" },
        { id: "files", label: "文件" },
      ],
      activeTabId: "terminal",
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorTabs>>,
  },
  {
    id: "disabled-browser",
    label: "禁用浏览器",
    props: {
      tabs: [
        { id: "changes", label: "改动", badge: 2 },
        { id: "browser", label: "浏览器", disabled: true },
        { id: "terminal", label: "终端" },
        { id: "files", label: "文件", badge: 8 },
      ],
      activeTabId: "files",
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorTabs>>,
  },
  {
    id: "long-labels",
    label: "长标签",
    props: {
      tabs: [
        { id: "changes", label: "本次改动摘要", badge: 12 },
        { id: "browser", label: "浏览器预览结果" },
        { id: "terminal", label: "终端输出记录" },
        { id: "files", label: "工作区文件树" },
      ],
      activeTabId: "changes",
    } as unknown as Partial<ComponentProps<typeof AwakenInspectorTabs>>,
  },
];
