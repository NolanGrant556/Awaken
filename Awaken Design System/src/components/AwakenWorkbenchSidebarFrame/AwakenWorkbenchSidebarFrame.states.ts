import type { ComponentProps } from "react";
import type { AwakenWorkbenchSidebarFrame } from "./AwakenWorkbenchSidebarFrame";

export type AwakenWorkbenchSidebarFrameState = {
  id: string;
  label: string;
  props: Partial<ComponentProps<typeof AwakenWorkbenchSidebarFrame>>;
};

export const AwakenWorkbenchSidebarFrameStates: AwakenWorkbenchSidebarFrameState[] = [
  {
    id: "sidebar-content",
    label: "侧栏内容",
    props: {
      children: "AwakenSidebar：仓库、会话和个人入口由父级提供。",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchSidebarFrame>>,
  },
  {
    id: "long-content",
    label: "较长侧栏内容",
    props: {
      children: "较长的侧栏框架内容：用于验证仓库列表、会话入口和用户区被包裹时仍然保持在统一边界内。",
    } as unknown as Partial<ComponentProps<typeof AwakenWorkbenchSidebarFrame>>,
  },
];
