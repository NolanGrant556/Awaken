import { describe, expect, it, vi } from "vitest";
import {
  enforceMainWindowZoom,
  MAIN_WINDOW_ZOOM_FACTOR,
  MAIN_WINDOW_ZOOM_LEVEL,
} from "./window-zoom.js";

describe("enforceMainWindowZoom", () => {
  it("restores layout zoom and disables visual zoom drift", async () => {
    const target = {
      setZoomLevel: vi.fn(),
      setZoomFactor: vi.fn(),
      setVisualZoomLevelLimits: vi.fn().mockResolvedValue(undefined),
    };

    await enforceMainWindowZoom(target);

    expect(target.setZoomLevel).toHaveBeenCalledWith(MAIN_WINDOW_ZOOM_LEVEL);
    expect(target.setZoomFactor).toHaveBeenCalledWith(MAIN_WINDOW_ZOOM_FACTOR);
    expect(target.setVisualZoomLevelLimits).toHaveBeenCalledWith(1, 1);
  });
});
