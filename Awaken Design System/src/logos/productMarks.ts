import type { AwakenLogoOpticalSize, AwakenLogoStyle } from "./construction";
import { AWAKEN_LOGO_STROKE } from "./construction";

export const PRODUCT_MARK_VIEWBOX: Record<AwakenLogoOpticalSize, string> = {
  16: "0 0 16 16",
  24: "0 0 24 24",
};

export const PRODUCT_MARK_PATHS: Record<
  AwakenLogoOpticalSize,
  Record<AwakenLogoStyle, readonly string[]>
> = {
  16: {
    filled: [
      "M7 1.25v4.2L3.9 10.75 1.25 14.7V10.8L4.55 5.55z",
      "M9 1.25v4.2l3.1 5.3 3.65 3.95V10.8L11.45 5.55z",
    ],
    outline: [
      "M7 1.5v3.9L4.05 10.55 1.55 14.35V10.7L4.7 5.65z",
      "M9 1.5v3.9l2.95 5.15 3.5 3.8V10.7L11.3 5.65z",
    ],
  },
  24: {
    filled: [
      "M10.9 2v6.3L6 16.1 2 22v-5.9L6.9 8.3z",
      "M13.1 2v6.3l4 7.8 4.9 5.9v-5.9L17.1 8.3z",
    ],
    outline: [
      "M10.9 2.4v5.7L6.2 15.8 2.5 21.4v-5.3L7.1 8.5z",
      "M13.1 2.4v5.7l3.8 7.4 4.6 5.6v-5.3L16.9 8.5z",
    ],
  },
};

export function productMarkStrokeWidth(
  opticalSize: AwakenLogoOpticalSize,
  style: AwakenLogoStyle,
): number | undefined {
  return style === "outline" ? AWAKEN_LOGO_STROKE[opticalSize] : undefined;
}
