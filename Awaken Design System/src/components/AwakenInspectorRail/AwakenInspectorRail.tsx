import "./AwakenInspectorRail.css";
import type { ReactNode } from "react";

export type AwakenInspectorRailProps = {
  title?: string;
  status?: string;
  tabs?: ReactNode;
  children: ReactNode;
};

export function AwakenInspectorRail({
  title = "Inspector",
  status,
  tabs,
  children,
}: AwakenInspectorRailProps) {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateRows: "auto auto minmax(0, 1fr)",
        gap: "14px",
        minWidth: 0,
        minHeight: 0,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: "12px",
          minWidth: 0,
        }}
      >
        <h2
          className="aw-truncate"
          title={title}
          style={{
            margin: 0,
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-base)",
            lineHeight: "var(--aw-line-height-base)",
            fontWeight: "var(--aw-font-weight-bold)",
          }}
        >
          {title}
        </h2>
        {status ? (
          <span
            style={{
              flex: "0 0 auto",
              color: "var(--aw-text-subtle)",
              fontSize: "var(--aw-font-size-xs)",
              fontWeight: "var(--aw-font-weight-semibold)",
              lineHeight: "var(--aw-line-height-xs)",
              whiteSpace: "nowrap",
            }}
          >
            {status}
          </span>
        ) : null}
      </div>
      {tabs ? <div>{tabs}</div> : null}
      <div
        style={{
          minWidth: 0,
          minHeight: 0,
          overflow: "auto",
        }}
      >
        {children}
      </div>
    </div>
  );
}
