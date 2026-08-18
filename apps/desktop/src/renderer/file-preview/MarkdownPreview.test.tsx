// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ProjectFilePreviewTextHandle } from "../../shared/runtime";
import MarkdownPreview from "./MarkdownPreview";

const preview: ProjectFilePreviewTextHandle = {
  previewId: "preview-md",
  requestId: "request-md",
  kind: "markdown",
  transport: "text",
  displayName: "README.md",
  relativePath: "README.md",
  byteLength: 16,
  content: "# Title\n\nHello",
  language: "markdown",
};

const frameProps = {
  displayName: "README.md",
  onReload: () => undefined,
  onOpenExternal: () => undefined,
  onReveal: () => undefined,
};

async function settleUntil(isReady: () => boolean): Promise<void> {
  await act(async () => {
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 10));
      if (isReady()) return;
    }
  });
}

async function setEditorValue(
  editor: HTMLTextAreaElement,
  value: string,
): Promise<void> {
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLTextAreaElement.prototype,
    "value",
  )?.set;
  await act(async () => {
    setter?.call(editor, value);
    editor.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

describe("MarkdownPreview", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("opens in preview mode and switches to an editable source view", async () => {
    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={preview}
          frameProps={frameProps}
          onSave={vi.fn(async () => ({ ok: true as const, byteLength: 16 }))}
        />,
      );
    });

    expect(container.querySelector("[data-preview-kind='markdown']")?.textContent).toContain(
      "Title",
    );
    expect(container.querySelector("[data-markdown-editor]")).toBeNull();

    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="编辑 Markdown"]')
        ?.click();
    });

    const editor = container.querySelector<HTMLTextAreaElement>(
      "[data-markdown-editor]",
    );
    expect(editor?.value).toBe("# Title\n\nHello");
    expect(container.querySelector("[data-preview-kind='markdown']")).toBeNull();
  });

  it("saves dirty Markdown when switching back to preview", async () => {
    const onSave = vi.fn(async (content: string) => ({
      ok: true as const,
      byteLength: content.length,
    }));

    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={preview}
          frameProps={frameProps}
          onSave={onSave}
        />,
      );
    });
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="编辑 Markdown"]')
        ?.click();
    });
    const editor = container.querySelector<HTMLTextAreaElement>(
      "[data-markdown-editor]",
    );
    if (!editor) throw new Error("missing editor");
    await setEditorValue(editor, "# Changed\n");
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="预览 Markdown"]')
        ?.click();
    });
    await settleUntil(
      () => container.querySelector("[data-preview-kind='markdown']") !== null,
    );

    expect(onSave).toHaveBeenCalledWith("# Changed\n");
    expect(
      container.querySelector("[data-preview-kind='markdown']")?.textContent,
    ).toContain("Changed");
    expect(container.querySelector("[data-markdown-editor]")).toBeNull();
  });

  it("keeps the editor open when save fails", async () => {
    const onSave = vi.fn(async () => ({
      ok: false as const,
      code: "FILE_UNWRITABLE" as const,
      message: "这个 Markdown 文件当前不可写入。",
    }));

    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={preview}
          frameProps={frameProps}
          onSave={onSave}
        />,
      );
    });
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="编辑 Markdown"]')
        ?.click();
    });
    const editor = container.querySelector<HTMLTextAreaElement>(
      "[data-markdown-editor]",
    );
    if (!editor) throw new Error("missing editor");
    await setEditorValue(editor, "# Locked\n");
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="预览 Markdown"]')
        ?.click();
    });
    await settleUntil(() => container.textContent?.includes("不可写入") === true);

    expect(container.querySelector("[data-markdown-editor]")).not.toBeNull();
    expect(container.querySelector("[role='alert']")?.textContent).toContain(
      "不可写入",
    );
  });

  it("saves with Ctrl/Cmd+S from the editor", async () => {
    const onSave = vi.fn(async (content: string) => ({
      ok: true as const,
      byteLength: content.length,
    }));

    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={preview}
          frameProps={frameProps}
          onSave={onSave}
        />,
      );
    });
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="编辑 Markdown"]')
        ?.click();
    });
    const editor = container.querySelector<HTMLTextAreaElement>(
      "[data-markdown-editor]",
    );
    if (!editor) throw new Error("missing editor");
    await setEditorValue(editor, "# Shortcut\n");
    await act(async () => {
      editor.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "s",
          ctrlKey: true,
          bubbles: true,
        }),
      );
    });
    await settleUntil(() => onSave.mock.calls.length > 0);

    expect(onSave).toHaveBeenCalledWith("# Shortcut\n");
  });
});
