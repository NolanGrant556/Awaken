import "./AwakenIcon.css";
import type { ReactNode } from "react";

export type AwakenIconProps = {
  size?: number;
  strokeWidth?: number;
  ariaHidden?: boolean;
  children: ReactNode;
};

export function AwakenIcon({ size = 18, strokeWidth = 1.85, ariaHidden = true, children }: AwakenIconProps) {
  return (
    <span
      className="aw-icon"
      aria-hidden={ariaHidden}
      style={{
        display: "inline-flex",
        flex: "0 0 auto",
        width: size,
        height: size,
        color: "currentColor",
      }}
    >
      <svg
        viewBox="0 0 24 24"
        style={{
          width: "100%",
          height: "100%",
          fill: "none",
          stroke: "currentColor",
          strokeWidth,
          strokeLinecap: "round",
          strokeLinejoin: "round",
        }}
      >
        {children}
      </svg>
    </span>
  );
}
