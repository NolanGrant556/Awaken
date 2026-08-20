"use client";

import type { HTMLAttributes } from "react";

import {
  AwakenThinkingOrb,
  type AwakenThinkingOrbMode,
  type AwakenThinkingOrbSize,
  type AwakenThinkingOrbTheme,
} from "./AwakenThinkingOrb";
import "./AwakenThinkingOrbStatus.css";

export interface AwakenThinkingOrbStatusProps
  extends Omit<HTMLAttributes<HTMLDivElement>, "children"> {
  label?: string;
  showEllipsis?: boolean;
  state?: AwakenThinkingOrbMode;
  size?: AwakenThinkingOrbSize;
  theme?: AwakenThinkingOrbTheme;
  speed?: number;
  paused?: boolean;
}

export function AwakenThinkingOrbStatus({
  label = "思考中",
  showEllipsis = true,
  state = "composing",
  size = 64,
  theme = "auto",
  speed,
  paused = false,
  className,
  role = "status",
  "aria-label": ariaLabel,
  ...props
}: AwakenThinkingOrbStatusProps) {
  const resolvedSpeed = speed ?? (state === "composing" ? 2.4 : 1);
  const resolvedAriaLabel = ariaLabel ?? `${label}${showEllipsis ? "..." : ""}`;

  return (
    <div
      {...props}
      className={["aw-thinking-orb-status", className].filter(Boolean).join(" ")}
      data-size={size}
      data-theme={theme}
      data-paused={paused || undefined}
      role={role}
      aria-label={resolvedAriaLabel}
    >
      <AwakenThinkingOrb
        state={state}
        size={size}
        theme={theme}
        speed={resolvedSpeed}
        paused={paused}
        aria-hidden="true"
        style={size === 64 ? { width: 56, height: 56 } : undefined}
      />
      <span className="aw-thinking-orb-status__label">{label}</span>
      {showEllipsis ? (
        <span className="aw-thinking-orb-status__ellipsis" aria-hidden="true">
          <span>.</span>
          <span>.</span>
          <span>.</span>
        </span>
      ) : null}
    </div>
  );
}
