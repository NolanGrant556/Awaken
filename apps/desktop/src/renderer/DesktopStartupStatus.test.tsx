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

it("keeps a restart error visible above the brand even if startup is still pending", async () => {
  vi.useFakeTimers();
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  const container = document.createElement("div");
  const root = createRoot(container);
  await act(async () => root.render(<DesktopStartupStatus starting needsRestart />));
  expect(container.querySelector(".is-brand[role=alert]")?.textContent)
    .toBe("Awaken 需要重新启动，请退出后重新打开。");
  expect(container.textContent).not.toContain("正在启动");
  await act(async () => vi.advanceTimersByTime(1000));
  expect(container.querySelector(".is-leaving")).toBeNull();
  expect(container.querySelector("[role=alert]")).not.toBeNull();
  await act(async () => root.render(<DesktopStartupStatus starting={false} needsRestart={false} />));
  await act(async () => vi.advanceTimersByTime(250));
  expect(container.textContent).toBe("");
  await act(async () => root.unmount());
});

it("cancels the ready fade when a late restart error arrives in the dock", async () => {
  vi.useFakeTimers();
  (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  const container = document.createElement("div");
  const root = createRoot(container);
  await act(async () => root.render(<DesktopStartupStatus starting placement="inline" />));
  await act(async () => root.render(<DesktopStartupStatus starting={false} placement="inline" />));
  await act(async () => vi.advanceTimersByTime(100));
  await act(async () => root.render(<DesktopStartupStatus starting={false} needsRestart placement="inline" />));
  await act(async () => vi.advanceTimersByTime(1000));
  expect(container.querySelector(".is-inline[role=alert]")).not.toBeNull();
  expect(container.querySelector(".is-leaving")).toBeNull();
  await act(async () => root.unmount());
});
