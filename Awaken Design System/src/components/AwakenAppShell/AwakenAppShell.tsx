import "./AwakenAppShell.css";
import type { CSSProperties, HTMLAttributes, ReactNode } from "react";
import type { PlatformKind } from "../../types";

export type AwakenAppShellProps = {
  sidebar: ReactNode;
  main: ReactNode;
  chrome?: ReactNode;
  sidebarCollapsed?: boolean;
  sidebarResizeHandleProps?: HTMLAttributes<HTMLDivElement>;
  platform?: PlatformKind;
};

export function AwakenAppShell({
  sidebar,
  main,
  chrome,
  sidebarCollapsed,
  sidebarResizeHandleProps,
  platform,
}: AwakenAppShellProps) {
  const sidebarResizeHandleClass = [
    "aw-app-sidebar-resize-handle",
    sidebarResizeHandleProps?.className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div
      className={`aw-app-shell aw-platform-${platform ?? "web"}${
        sidebarCollapsed ? " aw-app-shell-sidebar-collapsed" : ""
      }`}
      style={{
        display: "grid",
        gridTemplateColumns: sidebarCollapsed
          ? "0px minmax(0, 1fr)"
          : "var(--aw-sidebar-width) minmax(0, 1fr)",
        gridTemplateRows: chrome ? "var(--aw-chrome-height) minmax(0, 1fr)" : "minmax(0, 1fr)",
        height: "100%",
        minHeight: 0,
        overflow: "hidden",
        position: "relative",
        background: "var(--aw-bg-main)",
        transition: "grid-template-columns 210ms cubic-bezier(0.2, 0.8, 0.2, 1)",
      }}
    >
      {chrome ? (
        <div
          style={{
            gridColumn: "1 / -1",
            gridRow: "1",
            minWidth: 0,
            minHeight: 0,
            overflow: "hidden",
            position: "relative",
            zIndex: 2,
          }}
        >
          {chrome}
        </div>
      ) : null}
      <div
        className="aw-app-sidebar-slot"
        aria-hidden={sidebarCollapsed ? true : undefined}
        style={{
          gridColumn: "1",
          gridRow: chrome ? "1 / -1" : "1",
          minWidth: 0,
          minHeight: 0,
          overflow: "hidden",
          background: "var(--aw-bg-nav)",
          borderRight: "1px solid var(--aw-border-light)",
          position: "relative",
          zIndex: 1,
          "--aw-sidebar-top-inset": chrome
            ? "calc(var(--aw-chrome-height) + 4px)"
            : "8px",
        } as CSSProperties & { "--aw-sidebar-top-inset": string }}
      >
        {sidebar}
      </div>
      {sidebarResizeHandleProps && !sidebarCollapsed ? (
        <div
          {...sidebarResizeHandleProps}
          className={sidebarResizeHandleClass}
          style={sidebarResizeHandleProps.style}
        />
      ) : null}
      <div
        className="aw-app-main-slot"
        style={{
          gridColumn: "2",
          gridRow: chrome ? "2" : "1",
          minWidth: 0,
          minHeight: 0,
          overflow: "hidden",
          background: "var(--aw-bg-main)",
        }}
      >
        {main}
      </div>
    </div>
  );
}
