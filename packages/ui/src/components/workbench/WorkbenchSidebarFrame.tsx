import type { ReactNode } from "react";

type WorkbenchSidebarFrameProps = {
  children: ReactNode;
};

export function WorkbenchSidebarFrame({ children }: WorkbenchSidebarFrameProps) {
  return <div className="aw-workbench-sidebar-frame">{children}</div>;
}
