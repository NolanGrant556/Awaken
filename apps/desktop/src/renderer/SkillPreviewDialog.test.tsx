// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SkillPreviewResponse } from "../shared/runtime";
import { SkillPreviewDialog, type SkillPreviewTarget } from "./SkillPreviewDialog";

const presentation = {
  name: "Skill",
  sourceLabel: "全局",
  description: "用于验证统一的 Skill 预览信息区与文档阅读区。",
  enabled: true,
  deletable: true,
};

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

function assertSplitPreviewCards(root: ParentNode): void {
  const content = root.querySelector(".aw-skill-preview__content");
  const resourceCard = root.querySelector(".aw-skill-preview__card--resources");
  const documentCard = root.querySelector(".aw-skill-preview__card--document");
  expect(content).not.toBeNull();
  expect(resourceCard).not.toBeNull();
  expect(documentCard).not.toBeNull();
  expect(resourceCard?.parentElement).toBe(content);
  expect(documentCard?.parentElement).toBe(content);
  expect(resourceCard?.nextElementSibling).toBe(documentCard);
  expect(documentCard?.querySelector(".aw-skill-preview__card--resources")).toBeNull();
  expect(documentCard?.textContent).toContain("SKILL.md");
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
          presentation={presentation}
          runtimeClient={{
            getSkillPreview: vi.fn().mockResolvedValue({
              frontmatterRaw: "---\nname: skill\n---",
              bodyMarkdown: "# Skill body",
              resources: [
                { path: "SKILL.md", kind: "file", sizeBytes: 120 },
                { path: "references", kind: "directory", sizeBytes: 0 },
                { path: "references/guide.md", kind: "file", sizeBytes: 640 },
              ],
              resourcesTruncated: true,
            }),
          }}
          onEnabledChange={vi.fn()}
          onDelete={vi.fn()}
          onOpenChange={onOpenChange}
        />,
      );
    });

    const closeButton = document.querySelector<HTMLButtonElement>('button[aria-label="关闭"]');
    expect(closeButton).not.toBeNull();
    expect(document.querySelector(".aw-skill-preview-summary")?.textContent).toContain(
      "Skill全局",
    );
    expect(document.querySelector(".aw-skill-preview__delete")).not.toBeNull();
    expect(document.querySelector(".aw-skill-preview-document")).not.toBeNull();
    expect(document.querySelector(".aw-skill-preview__resources")?.textContent).toContain("包内文件");
    expect(document.querySelector(".aw-skill-preview__resources")?.textContent).toContain("仅显示前 512 项");
    expect(document.querySelector(".aw-skill-preview__resource-tree")?.textContent).toContain("guide.md");
    assertSplitPreviewCards(document);
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
          presentation={presentation}
          runtimeClient={{ getSkillPreview }}
          onOpenChange={onOpenChange}
        />,
      );
    });
    expect(document.querySelector(".aw-skill-preview__status")?.textContent).toContain(
      "正在读取 Skill 包",
    );
    expect(document.activeElement?.classList.contains("aw-skill-preview-document")).toBe(true);

    await act(async () => {
      root.render(
        <SkillPreviewDialog
          target={{ catalogEntryId: "second", projectRoot: "/workspace" }}
          presentation={presentation}
          runtimeClient={{ getSkillPreview }}
          onOpenChange={onOpenChange}
        />,
      );
    });
    await act(async () => {
      second.resolve({
        frontmatterRaw: "---\nname: second\n---",
        bodyMarkdown: "# Second body",
        resources: [
          { path: "SKILL.md", kind: "file", sizeBytes: 120 },
          { path: "notes", kind: "directory", sizeBytes: 0 },
        ],
      });
      await second.promise;
    });
    expect(document.body.textContent).toContain("Second body");
    assertSplitPreviewCards(document);

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
