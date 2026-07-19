// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SkillPreviewResponse } from "../shared/runtime";
import { SkillPreviewDialog, type SkillPreviewTarget } from "./SkillPreviewDialog";

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
};

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((nextResolve) => {
    resolve = nextResolve;
  });
  return { promise, resolve };
}

describe("SkillPreviewDialog", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    Object.defineProperty(HTMLElement.prototype, "scrollTo", {
      configurable: true,
      value: vi.fn(),
    });
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("keeps the close button accessible without visible close hints", async () => {
    const onOpenChange = vi.fn();

    await act(async () => {
      root.render(
        <SkillPreviewDialog
          target={{ catalogEntryId: "skill" }}
          runtimeClient={{
            getSkillPreview: vi.fn().mockResolvedValue({
              frontmatterRaw: "---\nname: skill\n---",
              bodyMarkdown: "# Skill body",
            }),
          }}
          onOpenChange={onOpenChange}
        />,
      );
    });

    const closeButton = document.querySelector<HTMLButtonElement>('button[aria-label="关闭"]');
    expect(closeButton).not.toBeNull();
    expect(closeButton?.getAttribute("title")).toBeNull();

    await act(async () => closeButton?.focus());
    expect(document.querySelector('[role="tooltip"]')).toBeNull();

    await act(async () => closeButton?.click());
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("focuses the single scroll region and ignores a stale preview response", async () => {
    const first = deferred<SkillPreviewResponse>();
    const second = deferred<SkillPreviewResponse>();
    const getSkillPreview = vi.fn((request: SkillPreviewTarget) =>
      request.catalogEntryId === "first" ? first.promise : second.promise,
    );
    const onOpenChange = vi.fn();

    await act(async () => {
      root.render(
        <SkillPreviewDialog
          target={{ catalogEntryId: "first" }}
          runtimeClient={{ getSkillPreview }}
          onOpenChange={onOpenChange}
        />,
      );
    });
    expect(document.activeElement?.classList.contains("skill-preview-dialog-scroll")).toBe(true);

    await act(async () => {
      root.render(
        <SkillPreviewDialog
          target={{ catalogEntryId: "second", projectRoot: "/workspace" }}
          runtimeClient={{ getSkillPreview }}
          onOpenChange={onOpenChange}
        />,
      );
    });
    await act(async () => {
      second.resolve({
        frontmatterRaw: "---\nname: second\n---",
        bodyMarkdown: "# Second body",
      });
      await second.promise;
    });
    expect(document.body.textContent).toContain("Second body");

    await act(async () => {
      first.resolve({
        frontmatterRaw: "---\nname: first\n---",
        bodyMarkdown: "# Stale first body",
      });
      await first.promise;
    });
    expect(document.body.textContent).toContain("Second body");
    expect(document.body.textContent).not.toContain("Stale first body");
    expect(getSkillPreview).toHaveBeenNthCalledWith(2, {
      catalogEntryId: "second",
      projectRoot: "/workspace",
    });

    await act(async () => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
