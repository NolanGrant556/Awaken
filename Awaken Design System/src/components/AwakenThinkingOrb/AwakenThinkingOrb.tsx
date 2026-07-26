"use client";

import { ThinkingOrb } from "thinking-orbs";
import type {
  OrbSize,
  OrbState,
  OrbTheme,
  ThinkingOrbProps,
} from "thinking-orbs";

export type AwakenThinkingOrbMode = OrbState;
export type AwakenThinkingOrbSize = OrbSize;
export type AwakenThinkingOrbTheme = OrbTheme;
export type AwakenThinkingOrbProps = ThinkingOrbProps;

export function AwakenThinkingOrb({ className, ...props }: AwakenThinkingOrbProps) {
  return (
    <ThinkingOrb
      {...props}
      className={["aw-thinking-orb", className].filter(Boolean).join(" ")}
    />
  );
}
