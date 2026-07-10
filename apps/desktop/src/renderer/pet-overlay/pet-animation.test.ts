import { describe, expect, it } from "vitest";
import {
  framePosition,
  frameDurationMs,
  totalCycleDurationMs,
  frameAtElapsedMs,
} from "./pet-animation";
import { PET_ROWS } from "../../shared/pets";

describe("framePosition", () => {
  it("computes Codex-compatible percentages", () => {
    expect(framePosition(0, 0)).toBe("0% 0%");
    expect(framePosition(0, 7)).toBe("100% 0%");
    expect(framePosition(8, 0)).toBe("0% 100%");
    expect(framePosition(8, 5)).toBe("71.42857142857143% 100%");
  });

  it("computes v2 row percentages against the 11-row atlas", () => {
    expect(framePosition(8, 0, 11)).toBe("0% 80%");
  });
});

describe("frameDurationMs", () => {
  it("slows idle by 6x", () => {
    expect(frameDurationMs("idle", 0)).toBe(PET_ROWS.idle.frameDurationsMs[0] * 6);
  });

  it("keeps other states at original speed", () => {
    expect(frameDurationMs("running", 0)).toBe(PET_ROWS.running.frameDurationsMs[0]);
  });
});

describe("frameAtElapsedMs", () => {
  it("loops idle forever", () => {
    const cycle = totalCycleDurationMs("idle");
    const first = frameAtElapsedMs("idle", 0);
    const afterCycle = frameAtElapsedMs("idle", cycle + 1);
    expect(first.row).toBe(afterCycle.row);
  });

  it("plays non-idle states for 3 cycles then enters the idle loop", () => {
    const state = "waving";
    const cycle = totalCycleDurationMs(state);
    const during = frameAtElapsedMs(state, 10);
    expect(during.column).toBe(0);

    const between2And3 = frameAtElapsedMs(state, cycle * 2.5);
    expect(between2And3.column).toBeLessThan(PET_ROWS[state].frames);

    const after3 = frameAtElapsedMs(state, cycle * 3 + 100);
    expect(after3.row).toBe(PET_ROWS.idle.row);
    expect(after3.column).toBe(0);
  });

  it("advances frames according to their durations", () => {
    const state = "idle";
    const duration0 = frameDurationMs(state, 0);
    const at0 = frameAtElapsedMs(state, 0);
    const atEndOfFirst = frameAtElapsedMs(state, duration0 - 1);
    const atStartOfSecond = frameAtElapsedMs(state, duration0 + 1);
    expect(at0.column).toBe(0);
    expect(atEndOfFirst.column).toBe(0);
    expect(atStartOfSecond.column).toBe(1);
  });

  it("uses correct row for each state", () => {
    expect(frameAtElapsedMs("idle", 0).row).toBe(PET_ROWS.idle.row);
    expect(frameAtElapsedMs("running-right", 0).row).toBe(PET_ROWS["running-right"].row);
    expect(frameAtElapsedMs("failed", 0).row).toBe(PET_ROWS.failed.row);
    expect(frameAtElapsedMs("review", 0).row).toBe(PET_ROWS.review.row);
  });
});
