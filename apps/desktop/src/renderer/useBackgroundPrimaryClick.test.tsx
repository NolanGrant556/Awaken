// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { AwakenWorkbenchSidebarFrame } from "@awaken/design-system";
import { expect, it, vi } from "vitest";
import { useBackgroundPrimaryClick } from "./useBackgroundPrimaryClick";

it("shares background clicks with the real sidebar frame while leaving controls, resize and drag alone", async () => {
  const callback = vi.fn();
  function Sidebar() {
    const events = useBackgroundPrimaryClick(callback);
    return <AwakenWorkbenchSidebarFrame {...events}>
      <div data-background>blank</div>
      <button>project</button>
      <label><span>search</span><input /></label>
      <div role="separator" />
      <div className="cm-editor"><div>editor whitespace</div></div>
    </AwakenWorkbenchSidebarFrame>;
  }
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const pointer = (target: Element, type: string, x = 20, button = 0) => {
    const event = new MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, button });
    Object.defineProperty(event, "pointerId", { value: 1 });
    target.dispatchEvent(event);
  };
  try {
    await act(async () => root.render(<Sidebar />));
    const background = host.querySelector("[data-background]")!;
    await act(async () => {
      for (const target of host.querySelectorAll("button, label span, input, [role=separator], .cm-editor div")) {
        pointer(target, "pointerdown"); pointer(target, "pointerup");
      }
      pointer(background, "pointerdown", 20, 2); pointer(background, "pointerup", 20, 2);
      pointer(background, "pointerdown"); pointer(background, "pointermove", 50); pointer(background, "pointerup", 50);
      pointer(background, "pointerdown"); pointer(background, "pointercancel"); pointer(background, "pointerup");
    });
    expect(callback).not.toHaveBeenCalled();
    await act(async () => { pointer(background, "pointerdown"); pointer(background, "pointerup"); });
    expect(callback).toHaveBeenCalledTimes(1);
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});
