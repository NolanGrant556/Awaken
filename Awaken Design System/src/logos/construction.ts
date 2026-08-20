export const AWAKEN_LOGO_OPTICAL_SIZES = [16, 24] as const;
export type AwakenLogoOpticalSize = (typeof AWAKEN_LOGO_OPTICAL_SIZES)[number];

export const AWAKEN_LOGO_STYLES = ["filled", "outline"] as const;
export type AwakenLogoStyle = (typeof AWAKEN_LOGO_STYLES)[number];

export const AWAKEN_LOGO_SHAPES = ["square", "circle", "horizontal", "vertical"] as const;
export type AwakenLogoOpticalShape = (typeof AWAKEN_LOGO_SHAPES)[number];

export const AWAKEN_LOGO_STROKE: Record<AwakenLogoOpticalSize, number> = {
  16: 1.25,
  24: 1.5,
};

export const AWAKEN_LOGO_CUT_MIN = 3;

export const AWAKEN_LOGO_DISPLAY_RANGE: Record<
  AwakenLogoOpticalSize,
  { min: number; max: number }
> = {
  16: { min: 12, max: 20 },
  24: { min: 22, max: 64 },
};

export function awakenLogoOpticalSizeForDisplay(px: number): AwakenLogoOpticalSize {
  return px <= AWAKEN_LOGO_DISPLAY_RANGE[16].max ? 16 : 24;
}
