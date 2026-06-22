import "./AwakenCenterStage.css";
import type { ReactNode } from "react";

export type AwakenCenterStageProps = {
  children: ReactNode;
  width?: number;
  topMargin?: number;
};

export function AwakenCenterStage({
  children,
  width = 768,
  topMargin = 96,
}: AwakenCenterStageProps) {
  return (
    <div
      style={{
        width: `${width}px`,
        maxWidth: "calc(100vw - var(--aw-sidebar-width) - 120px)",
        marginTop: `${topMargin}px`,
        marginLeft: "auto",
        marginRight: "auto",
        padding: "0 24px",
      }}
    >
      {children}
    </div>
  );
}
