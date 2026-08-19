/** @vitest-environment jsdom */

import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AwakenProjectTree } from "@awaken/design-system";

describe("conversation batch selection", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    host = document.createElement("div");
    document.body.appendChild(host);
    root = createRoot(host);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    host.remove();
    delete (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT;
  });

  it("toggles eligible rows and keeps running rows disabled", async () => {
    const onReadyChange = vi.fn();
    const onBusyChange = vi.fn();
    await act(async () => {
      root.render(
        <AwakenProjectTree
          items={[
            {
              id: "thread:ready",
              kind: "thread",
              label: "可删除会话",
              menuItems: [{ id: "delete", label: "删除会话" }],
              selection: { checked: false, onChange: onReadyChange },
            },
            {
              id: "thread:busy",
              kind: "thread",
              label: "运行中会话",
              isLoading: true,
              selection: {
                checked: false,
                disabled: true,
                disabledReason: "正在运行，停止后可删除",
                onChange: onBusyChange,
              },
            },
          ]}
        />,
      );
    });

    const readyRow = host.querySelector<HTMLButtonElement>(
      '[role="checkbox"][aria-label="选择 可删除会话"]',
    );
    const busyRow = host.querySelector<HTMLButtonElement>(
      '[role="checkbox"][aria-label="选择 运行中会话"]',
    );
    expect(readyRow).not.toBeNull();
    expect(busyRow?.getAttribute("aria-disabled")).toBe("true");
    expect(host.querySelector(".aw-project-row-actions")).toBeNull();

    await act(async () => readyRow?.click());
    await act(async () => busyRow?.click());

    expect(onReadyChange).toHaveBeenCalledWith(true);
    expect(onBusyChange).not.toHaveBeenCalled();
  });
});
