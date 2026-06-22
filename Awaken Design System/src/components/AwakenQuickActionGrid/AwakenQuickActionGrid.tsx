import "./AwakenQuickActionGrid.css";
import type { ReactNode } from "react";

export type AwakenQuickActionGridProps = {
  children: ReactNode;
};

export function AwakenQuickActionGrid({ children }: AwakenQuickActionGridProps) {
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
