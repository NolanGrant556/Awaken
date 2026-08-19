// @vitest-environment jsdom

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ProjectFilePreviewTextHandle } from "../../shared/runtime";
import MarkdownPreview, { markdownFromEditable } from "./MarkdownPreview";

const projectFileStyles = readFileSync(
  resolve(process.cwd(), "src/renderer/styles/project-files.css"),
  "utf8",
);

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
  editor: HTMLElement,
  html: string,
): Promise<void> {
  await act(async () => {
    editor.innerHTML = html;
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

  it("opens in preview mode and switches to the same formatted document in edit mode", async () => {
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

    const editor = container.querySelector<HTMLElement>(
      "[data-markdown-editor]",
    );
    expect(editor?.querySelector("h1")?.textContent).toBe("Title");
    expect(editor?.textContent).not.toContain("# Title");
    expect(editor?.getAttribute("data-markdown-mode")).toBe("edit");
    expect(editor?.classList.contains("project-file-markdown-preview")).toBe(true);
    expect(editor?.classList.contains("is-editing")).toBe(false);
    expect(container.querySelector(".project-file-markdown-mode-switch")).not.toBeNull();
    expect(container.querySelector(".project-file-markdown-save-state")).not.toBeNull();
  });

  it("preserves the content scroll position while switching preview and edit modes", async () => {
    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={preview}
          frameProps={frameProps}
          onSave={vi.fn(async () => ({ ok: true as const, byteLength: 16 }))}
        />,
      );
    });

    const previewArticle = container.querySelector<HTMLElement>(
      "[data-markdown-mode='preview']",
    );
    if (!previewArticle) throw new Error("missing preview article");
    previewArticle.scrollTop = 420;

    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="编辑 Markdown"]')
        ?.click();
    });
    const editor = container.querySelector<HTMLElement>(
      "[data-markdown-mode='edit']",
    );
    expect(editor?.scrollTop).toBe(420);
    if (!editor) throw new Error("missing editor");
    editor.scrollTop = 760;

    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="预览 Markdown"]')
        ?.click();
    });
    await settleUntil(
      () => container.querySelector("[data-markdown-mode='preview']") !== null,
    );

    expect(
      container.querySelector<HTMLElement>("[data-markdown-mode='preview']")
        ?.scrollTop,
    ).toBe(760);
  });

  it("does not restyle the whole document when editing", () => {
    const editorRule = projectFileStyles.match(
      /\.project-file-markdown-editor\s*\{([^}]*)\}/,
    )?.[1];

    expect(editorRule).toBeDefined();
    expect(editorRule).not.toMatch(
      /(?:background|box-shadow|font-size|line-height|padding|margin)\s*:/,
    );
    expect(projectFileStyles).not.toContain(
      ".project-file-markdown-editor.is-editing",
    );
    expect(projectFileStyles).toContain(
      ".project-file-markdown-mode-switch button[aria-pressed=\"true\"]",
    );
    expect(projectFileStyles).toMatch(
      /\.project-file-markdown-save-state:has\(\.project-file-markdown-preview__status\)\s*\{\s*padding-right:\s*8px;/,
    );
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
    const editor = container.querySelector<HTMLElement>(
      "[data-markdown-editor]",
    );
    if (!editor) throw new Error("missing editor");
    await setEditorValue(editor, "<h1>Changed</h1>");
    expect(
      container.querySelector(".project-file-markdown-save-state.is-dirty"),
    ).not.toBeNull();
    expect(container.querySelector("[role='status']")?.textContent).toBe(
      "未保存",
    );
    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('[aria-label="预览 Markdown"]')
        ?.click();
    });
    await settleUntil(
      () => container.querySelector("[data-markdown-mode='preview']") !== null,
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
    const editor = container.querySelector<HTMLElement>(
      "[data-markdown-editor]",
    );
    if (!editor) throw new Error("missing editor");
    await setEditorValue(editor, "<h1>Locked</h1>");
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
    const editor = container.querySelector<HTMLElement>(
      "[data-markdown-editor]",
    );
    if (!editor) throw new Error("missing editor");
    await setEditorValue(editor, "<h1>Shortcut</h1>");
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

  it("serializes formatted edits back to Markdown without exposing source markers", () => {
    const editor = document.createElement("article");
    editor.innerHTML =
      "<h2>Summary</h2><p><strong>Ready</strong> and <em>reviewed</em>.</p>" +
      "<ul><li>First</li><li>Second</li></ul>";

    expect(markdownFromEditable(editor)).toBe(
      "## Summary\n\n**Ready** and *reviewed*.\n\n- First\n- Second\n",
    );
  });
});
