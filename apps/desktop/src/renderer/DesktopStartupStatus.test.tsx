// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { DesktopStartupStatus } from "./DesktopStartupStatus";

afterEach(() => {
  vi.useRealTimers();
});
it("shows Chinese startup text, fades after ready, and remains absent on an already ready welcome", async () => {
  vi.useFakeTimers();
  (
    globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true;
  const container = document.createElement("div"),
    root = createRoot(container);
  await act(async () => root.render(<DesktopStartupStatus starting />));
  expect(container.textContent).toBe("正在启动 Awaken…");
  expect(container.querySelector(".is-brand")).not.toBeNull();
  await act(async () => root.render(<DesktopStartupStatus starting={false} />));
  expect(container.querySelector(".is-leaving")).not.toBeNull();
  await act(async () => vi.advanceTimersByTime(250));
  expect(container.textContent).toBe("");
  await act(async () => root.unmount());
  const readyRoot = createRoot(container);
  await act(async () =>
    readyRoot.render(<DesktopStartupStatus starting={false} />),
  );
  expect(container.textContent).toBe("");
  await act(async () => readyRoot.unmount());
});
