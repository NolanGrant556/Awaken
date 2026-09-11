/** @vitest-environment jsdom */
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ExpandedFileComposer } from "./ExpandedFileComposer";

describe("ExpandedFileComposer running pill", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    host.remove();
    delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean })
      .IS_REACT_ACT_ENVIRONMENT;
  });

  it("stops only from the dedicated stop button", async () => {
    const onStop = vi.fn();
    const onShowFull = vi.fn();
    await act(async () =>
      root.render(
        <ExpandedFileComposer
          active
          idleMode="compact"
          running
          completed={false}
          onShowCompact={() => undefined}
          onShowFull={onShowFull}
          onStop={onStop}
        >
          <div>原输入框</div>
        </ExpandedFileComposer>,
      ),
    );

    const expand = host.querySelector<HTMLButtonElement>(
      '[aria-label="正在执行，点击展开输入框"]',
    );
    const stop = host.querySelector<HTMLButtonElement>(
      '[aria-label="停止运行"]',
    );
    expect(expand).not.toBeNull();
    expect(stop).not.toBeNull();
    expect(host.textContent).not.toContain("点击停止");

    await act(async () => expand?.click());
    expect(onShowFull).toHaveBeenCalledTimes(1);
    expect(onStop).not.toHaveBeenCalled();

    await act(async () => stop?.click());
    expect(onStop).toHaveBeenCalledTimes(1);
    expect(onShowFull).toHaveBeenCalledTimes(1);
  });
});
