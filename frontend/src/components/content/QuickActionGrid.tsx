import type { ReactNode } from "react";

type QuickActionGridProps = {
  children: ReactNode;
};

export function QuickActionGrid({ children }: QuickActionGridProps) {
  return (
    <div
      style={{
        display: "flex",
        flexWrap: "wrap",
        gap: "8px",
        justifyContent: "center",
      }}
    >
      {children}
    </div>
  );
}
