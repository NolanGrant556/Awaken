// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AwakenDeliverableCard } from "@awaken/design-system";

describe("deliverable card behavior", () => {
  let host: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
  });

  it("mounts the reveal menu beside the card so local clipping boundaries can reposition it", () => {
    const onReveal = vi.fn();
    const onOpenExternal = vi.fn();
    act(() => {
      root.render(
        <AwakenDeliverableCard
          title="交付版本.pdf"
          extension="pdf"
          fileType="pdf"
          onOpen={() => undefined}
          onOpenExternal={onOpenExternal}
          onReveal={onReveal}
        />,
      );
    });

    expect(host.querySelector('[role="menu"]')).toBeNull();
    const trigger = host.querySelector<HTMLButtonElement>(
      '[aria-label="更多 交付版本.pdf 操作"]',
    );
    act(() => trigger?.click());

    const menu = host.querySelector<HTMLElement>('[role="menu"]');
    expect(menu).not.toBeNull();
    expect(menu?.closest(".aw-deliverable-card-menu")).not.toBeNull();
    expect(menu?.classList.contains("aw-popover__floating")).toBe(true);

    const actions = host.querySelectorAll<HTMLButtonElement>('[role="menuitem"]');
    expect(actions).toHaveLength(2);
    act(() => actions[0]?.click());
    expect(onOpenExternal).toHaveBeenCalledTimes(1);
    act(() => trigger?.click());
    const reveal = host.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')[1];
    act(() => reveal?.click());
    expect(onReveal).toHaveBeenCalledTimes(1);
    expect(host.querySelector('[role="menu"]')).toBeNull();
  });
});
