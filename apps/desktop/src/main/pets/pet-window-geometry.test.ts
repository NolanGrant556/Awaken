import { describe, expect, it } from "vitest";
import {
  PET_OVERLAY_MAX_HEIGHT,
  PET_OVERLAY_MAX_WIDTH,
  clampPetOverlayBoundsToWorkArea,
  normalizePetWindowBoundsForPlacement,
  resolveBottomAnchoredDragTop,
  resolvePetOverlayShape,
  resolvePetOverlayWindowSize,
  resolveSavedPetWindowBounds,
  resizePetWindowBoundsFromBottomRight,
  sanitizePetOverlayContentSize,
  stabilizePetWindowBounds,
} from "./pet-window-geometry.js";

describe("sanitizePetOverlayContentSize", () => {
  it("caps runaway renderer measurements to the pet overlay viewport", () => {
    expect(sanitizePetOverlayContentSize(3_660, 3_548)).toEqual({
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("uses a bounded fallback for invalid measurements", () => {
    expect(
      sanitizePetOverlayContentSize(Number.NaN, Number.POSITIVE_INFINITY, {
        width: 220,
        height: 260,
      }),
    ).toEqual({ width: 220, height: 260 });
  });
});

describe("resolvePetOverlayShape", () => {
  it("uses a slightly padded full-window interactive region", () => {
    expect(resolvePetOverlayShape(120, 127)).toEqual({
      x: 0,
      y: 0,
      width: 122,
      height: 129,
    });
  });

  it("does not grow beyond the fixed overlay window", () => {
    expect(resolvePetOverlayShape(PET_OVERLAY_MAX_WIDTH, PET_OVERLAY_MAX_HEIGHT)).toEqual({
      x: 0,
      y: 0,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});

describe("resolvePetOverlayWindowSize", () => {
  it("keeps an idle pet window close to the rendered pet bounds", () => {
    expect(resolvePetOverlayWindowSize(130, 144)).toEqual({
      width: 132,
      height: 146,
    });
  });
});

describe("pet overlay placement bounds", () => {
  it("stores a stable logical size even when Electron reports DPI-inflated bounds", () => {
    expect(
      normalizePetWindowBoundsForPlacement({
        x: 1307,
        y: 181,
        width: 477,
        height: 493,
      }),
    ).toEqual({
      x: 1307,
      y: 181,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("does not bottom-anchor saved DPI-inflated bounds on restore", () => {
    expect(
      resolveSavedPetWindowBounds({
        x: 1307,
        y: 181,
        width: 477,
        height: 493,
      }),
    ).toEqual({
      x: 1307,
      y: 181,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("bottom-anchors legacy runaway saved bounds on restore", () => {
    expect(
      resolveSavedPetWindowBounds({
        x: 1307,
        y: 181,
        width: 3816,
        height: 3706,
      }),
    ).toEqual({
      x: 1307 + 3816 - PET_OVERLAY_MAX_WIDTH,
      y: 181 + 3706 - PET_OVERLAY_MAX_HEIGHT,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});

describe("resolveBottomAnchoredDragTop", () => {
  it("keeps the bottom edge stable when the observed window height grows", () => {
    expect(resolveBottomAnchoredDragTop(181, 403, 0, 452)).toBe(132);
  });

  it("applies pointer y movement relative to the stable bottom edge", () => {
    expect(resolveBottomAnchoredDragTop(181, 403, 20, 452)).toBe(152);
  });
});

describe("stabilizePetWindowBounds", () => {
  it("shrinks a drifted overlay window while preserving the bottom-right corner", () => {
    expect(
      stabilizePetWindowBounds({
        x: 1612,
        y: 507,
        width: 488,
        height: 525,
      }),
    ).toEqual({
      x: 1716,
      y: 632,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});

describe("resizePetWindowBoundsFromBottomRight", () => {
  it("resizes the overlay without moving the pet anchor", () => {
    expect(
      resizePetWindowBoundsFromBottomRight(
        { x: 100, y: 100, width: 384, height: 400 },
        { width: 132, height: 146 },
      ),
    ).toEqual({
      x: 352,
      y: 354,
      width: 132,
      height: 146,
    });
  });
});

describe("clampPetOverlayBoundsToWorkArea", () => {
  it("keeps the pet overlay inside the display work area", () => {
    expect(
      clampPetOverlayBoundsToWorkArea(
        { x: 1662, y: 759, width: PET_OVERLAY_MAX_WIDTH, height: PET_OVERLAY_MAX_HEIGHT },
        { x: 0, y: 0, width: 2048, height: 1104 },
      ),
    ).toEqual({
      x: 1662,
      y: 704,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });

  it("allows transparent window margins outside the display when the visible region stays inside", () => {
    expect(
      clampPetOverlayBoundsToWorkArea(
        { x: 2000, y: -400, width: PET_OVERLAY_MAX_WIDTH, height: PET_OVERLAY_MAX_HEIGHT },
        { x: 0, y: 0, width: 2048, height: 1104 },
        { x: 0, y: 270, width: 120, height: 130 },
      ),
    ).toEqual({
      x: 1928,
      y: -270,
      width: PET_OVERLAY_MAX_WIDTH,
      height: PET_OVERLAY_MAX_HEIGHT,
    });
  });
});
