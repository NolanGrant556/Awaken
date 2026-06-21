import type { ReactNode } from "react";

type TopBarProps = {
  children?: ReactNode;
};

export function TopBar({ children }: TopBarProps) {
  return (
    <header
      aria-label="顶部操作栏"
      style={{
        position: "sticky",
        top: 0,
        zIndex: 2,
        display: "flex",
        alignItems: "center",
        justifyContent: "flex-end",
        gap: "12px",
        height: "44px",
        padding: "8px 16px 8px 12px",
        background: "var(--aw-bg-main)",
        borderBottom: "1px solid var(--aw-border-light)",
      }}
    >
      {children}
    </header>
  );
}
