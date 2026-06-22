import "./AwakenWorkbenchSidebarFrame.css";
import type { ReactNode } from "react";

export type AwakenWorkbenchSidebarFrameProps = {
  children: ReactNode;
};

export function AwakenWorkbenchSidebarFrame({ children }: AwakenWorkbenchSidebarFrameProps) {
  return <div className="aw-workbench-sidebar-frame">{children}</div>;
}
