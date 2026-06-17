import type { ReactNode } from "react";
import type { PlatformKind } from "../../types";

type AppShellProps = {
  sidebar: ReactNode;
  main: ReactNode;
  platform?: PlatformKind;
};

export function AppShell({ sidebar, main, platform }: AppShellProps) {
  return (
    <div
      className={`aw-app-shell aw-platform-${platform ?? "web"}`}
      style={{
        display: "grid",
        gridTemplateColumns: "var(--aw-sidebar-width) minmax(0, 1fr)",
        height: "100%",
        minHeight: 0,
        overflow: "hidden",
        background: "var(--aw-bg-main)",
      }}
    >
      <div
        style={{
          minWidth: 0,
          minHeight: 0,
          overflow: "hidden",
          background: "var(--aw-bg-nav)",
          borderRight: "1px solid var(--aw-border-light)",
        }}
      >
        {sidebar}
      </div>
      <div
        style={{
          minWidth: 0,
          minHeight: 0,
          overflow: "auto",
          background: "var(--aw-bg-main)",
        }}
      >
        {main}
      </div>
    </div>
  );
}
