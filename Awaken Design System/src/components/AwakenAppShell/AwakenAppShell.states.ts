import type { ComponentProps } from "react";
import type { AwakenAppShell } from "./AwakenAppShell";

export type AwakenAppShellState = {
  id: string;
  label: string;
  props: ComponentProps<typeof AwakenAppShell>;
};

export const AwakenAppShellStates: AwakenAppShellState[] = [
  {
    id: "desktop-shell",
    label: "桌面双栏骨架",
    props: {
      sidebar: "左侧导航、会话列表和项目入口",
      main: "运行工作台主区域，承载当前任务的执行内容",
      chrome: "工作台顶部控制和全局状态区",
      sidebarCollapsed: false,
      platform: "macos",
    },
  },
  {
    id: "sidebar-collapsed",
    label: "侧边栏收起",
    props: {
      sidebar: "收起后不可见的导航内容",
      main: "主区域接管剩余宽度，用于专注查看执行结果",
      chrome: "工作台顶部控制保留全局上下文",
      sidebarCollapsed: true,
      platform: "macos",
    },
  },
  {
    id: "web-shell",
    label: "无顶部 Chrome",
    props: {
      sidebar: "Web 侧栏导航",
      main: "嵌入式页面主体",
      platform: "web",
    },
  },
];
