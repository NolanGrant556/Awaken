import { describe, expect, it, vi } from "vitest";
import {
  safelySendToRenderer,
  shouldRecoverMainRenderer,
} from "./renderer-delivery.js";

describe("renderer delivery", () => {
  it("returns false instead of throwing after the renderer frame is disposed", () => {
    const target = {
      isDestroyed: () => false,
      send: vi.fn(() => {
        throw new Error("Render frame was disposed");
      }),
    };

    expect(safelySendToRenderer(target, "awaken:event", {})).toBe(false);
    expect(target.send).toHaveBeenCalledTimes(1);
  });

  it("does not send to missing or destroyed targets", () => {
    expect(safelySendToRenderer(undefined, "awaken:event", {})).toBe(false);
    const target = { isDestroyed: () => true, send: vi.fn() };
    expect(safelySendToRenderer(target, "awaken:event", {})).toBe(false);
    expect(target.send).not.toHaveBeenCalled();
  });

  it("allows one bounded recovery for an abnormal renderer exit", () => {
    expect(shouldRecoverMainRenderer("oom", 0)).toBe(true);
    expect(shouldRecoverMainRenderer("crashed", 1)).toBe(false);
    expect(shouldRecoverMainRenderer("clean-exit", 0)).toBe(false);
  });
});
