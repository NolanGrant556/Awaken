import type { CSSProperties } from "react";
import { AWAKEN_ICON_OPTICAL_SIZES, type AwakenIconOpticalSize } from "./types";

export const AWAKEN_ICON_STROKE_TOKEN = {
  16: "var(--aw-icon-stroke-16)",
  24: "var(--aw-icon-stroke-24)",
} as const;

export function resolveAwakenIconOpticalSize(
  size: number | string | undefined,
): AwakenIconOpticalSize {
  const numeric = typeof size === "number" ? size : Number.parseFloat(String(size ?? ""));
  if (!Number.isFinite(numeric)) {
    return 16;
  }
  return numeric >= 22 ? 24 : 16;
}

export function awakenIconViewBox(opticalSize: AwakenIconOpticalSize): string {
  return `0 0 ${opticalSize} ${opticalSize}`;
}

export function projectIconPointTo16(value: number): number {
  return 2 + ((value - 3) * 12) / 18;
}

export function awakenIconStrokeStyle(
  opticalSize: AwakenIconOpticalSize,
): CSSProperties {
  return {
    ["--aw-icon-stroke" as string]: AWAKEN_ICON_STROKE_TOKEN[opticalSize],
  };
}

export function isAwakenIconOpticalSize(value: number): value is AwakenIconOpticalSize {
  return (AWAKEN_ICON_OPTICAL_SIZES as readonly number[]).includes(value);
}
