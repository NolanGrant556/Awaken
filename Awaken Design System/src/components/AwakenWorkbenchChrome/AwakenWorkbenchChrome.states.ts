import type { ComponentProps } from "react";
import type { AwakenWorkbenchChrome } from "./AwakenWorkbenchChrome";

export type AwakenWorkbenchChromeState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWorkbenchChrome>>;
};

const noop = () => {};

export const AwakenWorkbenchChromeStates: AwakenWorkbenchChromeState[] = [
  {
    id: "expanded-sidebar",
    label: "侧栏展开",
    props: {
      sidebarCollapsed: false,
      onToggleSidebar: noop,
      onNewTask: noop,
      right: "Local runtime / Awaken",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
  {
    id: "collapsed-sidebar",
    label: "侧栏折叠",
    props: {
      sidebarCollapsed: true,
      onToggleSidebar: noop,
      onNewTask: noop,
      right: "Choose workspace",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
  {
    id: "starting",
    label: "启动状态",
    props: {
      sidebarCollapsed: false,
      onToggleSidebar: noop,
      onNewTask: noop,
      right: "Starting Awaken / runtime 准备中",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
  {
    id: "long-content",
    label: "较长右侧状态",
    props: {
      sidebarCollapsed: false,
      onToggleSidebar: noop,
      onNewTask: noop,
      right: "当前工作区路径等待校验：/Users/nolangrant/Awaken",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
];
