// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  formatRunElapsedTime,
  resolvedRunElapsedMs,
  RunElapsedTime,
} from "./RunElapsedTime";

describe("RunElapsedTime", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    vi.useFakeTimers();
    vi.setSystemTime(10_000);
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    vi.useRealTimers();
  });

  it("appears immediately and advances in place while the run is active", async () => {
    await act(async () => {
      root.render(<RunElapsedTime startedAtMs={10_000} running />);
    });
    const timer = container.querySelector('[role="timer"]');
    expect(timer?.tagName).toBe("SPAN");
    expect(timer?.textContent).toBe("执行时间 0 秒");

    await act(async () => {
      vi.advanceTimersByTime(3_000);
    });
    expect(timer?.textContent).toBe("执行时间 3 秒");
  });

  it("freezes on durable elapsed time after completion", async () => {
    await act(async () => {
      root.render(<RunElapsedTime startedAtMs={10_000} elapsedMs={65_000} />);
    });
    expect(container.textContent).toBe("执行时间 1 分 05 秒");

    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    expect(container.textContent).toBe("执行时间 1 分 05 秒");
  });

  it("prefers replay-safe elapsed time and otherwise resolves a terminal boundary", () => {
    expect(
      resolvedRunElapsedMs({
        startedAtMs: 1_000,
        completedAtMs: 9_000,
        elapsedMs: 5_000,
        running: false,
        nowMs: 20_000,
      }),
    ).toBe(5_000);
    expect(
      resolvedRunElapsedMs({
        startedAtMs: 1_000,
        completedAtMs: 9_000,
        running: false,
        nowMs: 20_000,
      }),
    ).toBe(8_000);
    expect(formatRunElapsedTime(3_661_000)).toBe("1 小时 1 分 1 秒");
  });
});
