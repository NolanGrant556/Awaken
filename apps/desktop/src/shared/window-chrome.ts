export const MACOS_WINDOW_CHROME_GEOMETRY = {
  chromeHeightPx: 44,
  trafficLightPosition: { x: 18, y: 14 },
  sidebarSpacerPx: 88,
  // Must stay an integer CSS pixel: fractional offsets (the previous -0.75)
  // put the 44px-bar icons on quarter/half device pixels, so any
  // re-rasterization (layer promotion, panel animation) visibly shifts the
  // stroke anti-aliasing on Retina displays.
  controlTopOffsetPx: -1,
} as const;
