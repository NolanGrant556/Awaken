import type { ReactNode } from "react";

type AppShellProps = {
  sidebar: ReactNode;
  main: ReactNode;
};

export function AppShell({ sidebar, main }: AppShellProps) {
  return (
    <div className="app-stage">
      <div className="desktop-window">
        <aside className="window-sidebar">{sidebar}</aside>
        <main className="window-main">{main}</main>
      </div>
    </div>
  );
}
