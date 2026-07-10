import type { Rectangle } from "electron";

export const PET_OVERLAY_MAX_WIDTH = 384;
export const PET_OVERLAY_MAX_HEIGHT = 400;

export const fixedOverlaySize = {
  width: PET_OVERLAY_MAX_WIDTH,
  height: PET_OVERLAY_MAX_HEIGHT,
};

export const minOverlaySize = { width: 120, height: 120 };

const legacyOversizedPlacementFactor = 1.5;
const shapeClipSafetyPx = 2;

export type OverlaySize = {
  width: number;
  height: number;
};

export function sanitizePetOverlayContentSize(
  width: number,
  height: number,
  fallback: { width: number; height: number } = minOverlaySize,
): OverlaySize {
  return {
    width: Math.max(
      minOverlaySize.width,
      Math.min(
        PET_OVERLAY_MAX_WIDTH,
        Math.round(Number.isFinite(width) ? width : fallback.width),
      ),
    ),
    height: Math.max(
      minOverlaySize.height,
      Math.min(
        PET_OVERLAY_MAX_HEIGHT,
        Math.round(Number.isFinite(height) ? height : fallback.height),
      ),
    ),
  };
}

export function resolvePetOverlayWindowSize(
  width: number,
  height: number,
): OverlaySize {
  const contentSize = sanitizePetOverlayContentSize(width, height);
  return {
    width: Math.min(PET_OVERLAY_MAX_WIDTH, contentSize.width + shapeClipSafetyPx),
    height: Math.min(PET_OVERLAY_MAX_HEIGHT, contentSize.height + shapeClipSafetyPx),
  };
}

export function resolvePetOverlayShape(
  width: number,
  height: number,
): Rectangle {
  const windowSize = resolvePetOverlayWindowSize(width, height);
  return {
    x: 0,
    y: 0,
    width: windowSize.width,
    height: windowSize.height,
  };
}

export function normalizePetWindowBoundsForPlacement(bounds: Rectangle): Rectangle {
  const size = sanitizePetOverlayContentSize(bounds.width, bounds.height);
  return {
    x: bounds.x,
    y: bounds.y,
    ...size,
  };
}

export function resolveSavedPetWindowBounds(savedBounds: Rectangle): Rectangle {
  const isLegacyRunawayBounds =
    savedBounds.width > fixedOverlaySize.width * legacyOversizedPlacementFactor ||
    savedBounds.height > fixedOverlaySize.height * legacyOversizedPlacementFactor;

  if (isLegacyRunawayBounds) {
    return {
      x: savedBounds.x + savedBounds.width - fixedOverlaySize.width,
      y: savedBounds.y + savedBounds.height - fixedOverlaySize.height,
      ...fixedOverlaySize,
    };
  }

  const size = sanitizePetOverlayContentSize(savedBounds.width, savedBounds.height);
  return {
    x: savedBounds.x,
    y: savedBounds.y,
    ...size,
  };
}

export function resolveBottomAnchoredDragTop(
  originWindowY: number,
  originWindowHeight: number,
  dy: number,
  observedWindowHeight: number,
): number {
  return Math.round(originWindowY + originWindowHeight + dy - observedWindowHeight);
}

export function stabilizePetWindowBounds(
  bounds: Rectangle,
  targetSize: OverlaySize = fixedOverlaySize,
): Rectangle {
  return {
    x: bounds.x + bounds.width - targetSize.width,
    y: bounds.y + bounds.height - targetSize.height,
    ...targetSize,
  };
}

export function clampPetOverlayBoundsToWorkArea(
  bounds: Rectangle,
  displayBounds: Rectangle,
  visibleRegion: Rectangle = { x: 0, y: 0, width: bounds.width, height: bounds.height },
): Rectangle {
  const size = sanitizePetOverlayContentSize(bounds.width, bounds.height);
  const width = Math.min(size.width, displayBounds.width);
  const height = Math.min(size.height, displayBounds.height);
  const visibleWidth = Math.min(visibleRegion.width, displayBounds.width);
  const visibleHeight = Math.min(visibleRegion.height, displayBounds.height);
  const minX = displayBounds.x - visibleRegion.x;
  const maxX = displayBounds.x + displayBounds.width - visibleRegion.x - visibleWidth;
  const minY = displayBounds.y - visibleRegion.y;
  const maxY = displayBounds.y + displayBounds.height - visibleRegion.y - visibleHeight;
  const x = Math.min(
    Math.max(bounds.x, Math.min(minX, maxX)),
    Math.max(minX, maxX),
  );
  const y = Math.min(
    Math.max(bounds.y, Math.min(minY, maxY)),
    Math.max(minY, maxY),
  );
  return { x, y, width, height };
}

export function resizePetWindowBoundsFromBottomRight(
  bounds: Rectangle,
  nextSize: OverlaySize,
): Rectangle {
  return {
    x: bounds.x + bounds.width - nextSize.width,
    y: bounds.y + bounds.height - nextSize.height,
    ...nextSize,
  };
}
