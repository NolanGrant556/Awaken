// @vitest-environment jsdom

import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DesktopErrorBoundary } from "./DesktopErrorBoundary";

function BrokenView(): never {
  throw new Error("render failed");
}

describe("DesktopErrorBoundary", () => {
  afterEach(() => vi.restoreAllMocks());

  it("keeps a recoverable product surface instead of a blank window", () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    const root = createRoot(container);
    vi.spyOn(console, "error").mockImplementation(() => undefined);

    act(() => {
      root.render(
        <DesktopErrorBoundary>
          <BrokenView />
        </DesktopErrorBoundary>,
      );
    });

    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      "界面暂时无法继续显示",
    );
    expect(container.querySelector("button")?.textContent).toBe("重新加载界面");

    act(() => root.unmount());
    container.remove();
  });
});
