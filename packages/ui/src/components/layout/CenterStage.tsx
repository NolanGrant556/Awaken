import type { ReactNode } from "react";

type CenterStageProps = {
  children: ReactNode;
  width?: number;
  topMargin?: number;
};

export function CenterStage({
  children,
  width = 768,
  topMargin = 96,
}: CenterStageProps) {
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
