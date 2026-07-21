import type { ComponentProps } from "react";
import type { AwakenWorkbenchChrome } from "./AwakenWorkbenchChrome";

export type AwakenWorkbenchChromeState = {
  id: string;
  label: string;
  description: string;
  props: Partial<ComponentProps<typeof AwakenWorkbenchChrome>>;
};

const noop = () => {};

export const AwakenWorkbenchChromeStates: AwakenWorkbenchChromeState[] = [
  {
    id: "expanded-sidebar",
    label: "主窗口：左侧栏展开",
    description: "顶部左侧只保留侧栏开关；右侧区域由 Desktop 放入 Inspector 开关。",
    props: {
      sidebarCollapsed: false,
      onToggleSidebar: noop,
      onNewTask: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
  {
    id: "collapsed-sidebar",
    label: "主窗口：左侧栏收起",
    description: "侧栏收起后，顶部左侧会多出新建任务按钮，用户无需先展开侧栏。",
    props: {
      sidebarCollapsed: true,
      onToggleSidebar: noop,
      onNewTask: noop,
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
  {
    id: "starting",
    label: "Awaken 正在启动",
    description: "Desktop 启动本地服务期间，右侧状态位显示 Starting。",
    props: {
      sidebarCollapsed: false,
      onToggleSidebar: noop,
      onNewTask: noop,
      right: "Starting",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
  {
    id: "needs-restart",
    label: "Awaken 需要重启",
    description: "配置变更需要重新启动本地服务时，右侧状态位显示 Needs restart。",
    props: {
      sidebarCollapsed: false,
      onToggleSidebar: noop,
      onNewTask: noop,
      right: "Needs restart",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchChrome>>,
  },
];
