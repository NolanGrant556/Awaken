import "./AwakenWorkbenchSidebarFrame.css";
import type { HTMLAttributes, ReactNode } from "react";

export type AwakenWorkbenchSidebarFrameProps = HTMLAttributes<HTMLDivElement> & {
  children: ReactNode;
};

export function AwakenWorkbenchSidebarFrame({ children, className, ...props }: AwakenWorkbenchSidebarFrameProps) {
  return <div {...props} className={`aw-workbench-sidebar-frame${className ? ` ${className}` : ""}`}>{children}</div>;
}
