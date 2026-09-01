// @vitest-environment jsdom

import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { AwakenComposerBar } from "@awaken/design-system";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

function ContextUsageHarness() {
  const [panelOpen, setPanelOpen] = useState(false);

  return (
    <>
      <AwakenComposerBar
        value=""
        onChange={vi.fn()}
        variant="dock"
        metaItems={[
          {
            id: "context",
            label: "上下文窗口",
            value: "29K / 267K",
            progress: 29_000 / 267_000,
            onClick: () => setPanelOpen((current) => !current),
            ariaHaspopup: "dialog",
            ariaExpanded: panelOpen,
            ariaControls: "context-usage-test-panel",
          },
        ]}
      />
      {panelOpen ? (
        <button
          type="button"
          aria-label="关闭测试上下文面板"
          onClick={() => setPanelOpen(false)}
        />
      ) : null}
    </>
  );
}

function movePointerInto(
  element: Element,
  relatedTarget: EventTarget = document.body,
): void {
  element.dispatchEvent(
    new MouseEvent("pointerover", { bubbles: true, relatedTarget }),
  );
  element.parentElement?.dispatchEvent(
    new MouseEvent("mouseenter", { bubbles: false, relatedTarget }),
  );
}

function movePointerOutOf(
  element: Element,
  relatedTarget: EventTarget,
): void {
  element.dispatchEvent(
    new MouseEvent("pointerout", { bubbles: true, relatedTarget }),
  );
  element.parentElement?.dispatchEvent(
    new MouseEvent("mouseleave", { bubbles: false, relatedTarget }),
  );
}

describe("AwakenComposerBar context usage tooltip", () => {
  let host: HTMLDivElement;
  let root: Root | undefined;

  beforeEach(() => {
    (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    host = document.createElement("div");
    document.body.append(host);
    root = createRoot(host);
    act(() => root?.render(<ContextUsageHarness />));
  });

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
      root = undefined;
    }
    host.remove();
    delete (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT?: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT;
  });

  it("shows only while hovered after opening and closing the context panel", () => {
    const trigger = host.querySelector<HTMLButtonElement>(
      ".composer-bar-meta-item-meter",
    );
    expect(trigger).not.toBeNull();

    act(() => movePointerInto(trigger!));
    expect(document.body.querySelector('[role="tooltip"]')?.textContent).toContain(
      "11% 当前使用",
    );

    act(() => trigger?.click());
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull();

    const closeButton = host.querySelector<HTMLButtonElement>(
      '[aria-label="关闭测试上下文面板"]',
    );
    expect(closeButton).not.toBeNull();

    act(() => movePointerOutOf(trigger!, closeButton!));
    act(() => closeButton?.click());

    act(() => movePointerInto(trigger!, closeButton!));
    expect(document.body.querySelector('[role="tooltip"]')?.textContent).toContain(
      "11% 当前使用",
    );

    act(() => movePointerOutOf(trigger!, document.body));
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull();
  });

  it("waits for a fresh hover cycle when the panel closes under the pointer", () => {
    const trigger = host.querySelector<HTMLButtonElement>(
      ".composer-bar-meta-item-meter",
    );
    expect(trigger).not.toBeNull();

    act(() => movePointerInto(trigger!));
    expect(document.body.querySelector('[role="tooltip"]')).not.toBeNull();

    act(() => trigger?.click());
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull();

    act(() => trigger?.click());
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull();

    act(() => movePointerOutOf(trigger!, document.body));
    expect(document.body.querySelector('[role="tooltip"]')).toBeNull();

    act(() => movePointerInto(trigger!));
    expect(document.body.querySelector('[role="tooltip"]')?.textContent).toContain(
      "11% 当前使用",
    );
  });
});
