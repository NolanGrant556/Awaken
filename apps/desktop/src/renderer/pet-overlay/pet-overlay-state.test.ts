import { describe, expect, it } from "vitest";
import { resolveOverlayAnimationState } from "./pet-overlay-state";

describe("resolveOverlayAnimationState", () => {
  it("uses the business state when idle", () => {
    expect(
      resolveOverlayAnimationState({
        resizeMode: false,
        dragging: false,
        dragDirection: "right",
        hover: false,
        effectiveBusinessState: "waiting",
      }),
    ).toBe("waiting");
  });

  it("uses jumping while hovering over an otherwise idle pet", () => {
    expect(
      resolveOverlayAnimationState({
        resizeMode: false,
        dragging: false,
        dragDirection: "right",
        hover: true,
        effectiveBusinessState: "idle",
      }),
    ).toBe("jumping");
  });

  it("uses directional running while dragging", () => {
    expect(
      resolveOverlayAnimationState({
        resizeMode: false,
        dragging: true,
        dragDirection: "left",
        hover: true,
        effectiveBusinessState: "running",
      }),
    ).toBe("running-left");

    expect(
      resolveOverlayAnimationState({
        resizeMode: false,
        dragging: true,
        dragDirection: "right",
        hover: true,
        effectiveBusinessState: "running",
      }),
    ).toBe("running-right");
  });

  it("keeps the current business state while resizing", () => {
    expect(
      resolveOverlayAnimationState({
        resizeMode: true,
        dragging: true,
        dragDirection: "left",
        hover: true,
        effectiveBusinessState: "review",
      }),
    ).toBe("review");
  });
});
