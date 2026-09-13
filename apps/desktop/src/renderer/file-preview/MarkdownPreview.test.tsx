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
  byteLength: 28,
  content: "# Title\n\nFirst paragraph.\n",
  contentDigest: "a".repeat(64),
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
    for (let attempt = 0; attempt < 50; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 20));
      if (isReady()) return;
    }
  });
}

describe("MarkdownPreview", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("defaults to preview and switches to editable Markdown source with one text action", async () => {
    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={preview}
          frameProps={frameProps}
          onSave={vi.fn(async () => ({ ok: true as const, byteLength: 28 }))}
        />,
      );
    });

    expect(container.querySelector("[data-markdown-mode='preview']")?.textContent).toContain("Title");
    expect(container.querySelector(".cm-editor")).toBeNull();
    const toggle = [...container.querySelectorAll("button")].find(
      (button) => button.textContent === "查看源代码",
    );
    expect(toggle).toBeDefined();
    expect(container.querySelector('[aria-label="保存 Markdown"]')).toBeNull();
    expect(container.querySelector('[aria-label="重新载入文件预览"]')).toBeNull();

    await act(async () => toggle?.click());
    await settleUntil(() => container.querySelector(".cm-editor") !== null);
    expect(container.querySelector(".cm-editor")).not.toBeNull();
    expect(container.textContent).toContain("查看预览");
  });

  it("annotates rendered blocks with source lines for preview selection actions", async () => {
    await act(async () => {
      root.render(<MarkdownPreview preview={preview} frameProps={frameProps} />);
    });

    const heading = container.querySelector("h1");
    const paragraph = container.querySelector("p");
    expect(heading?.getAttribute("data-source-start-line")).toBe("1");
    expect(paragraph?.getAttribute("data-source-start-line")).toBe("3");
  });

  it("adds a rendered Markdown selection to the conversation with canonical source lines", async () => {
    const onAddTextSelection = vi.fn();
    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={preview}
          frameProps={frameProps}
          onAddTextSelection={onAddTextSelection}
        />,
      );
    });

    const paragraph = container.querySelector("p");
    const textNode = paragraph?.firstChild;
    expect(textNode).toBeDefined();
    const range = document.createRange();
    Object.defineProperty(range, "getBoundingClientRect", {
      configurable: true,
      value: () => ({
        bottom: 24,
        height: 16,
        left: 12,
        right: 120,
        top: 8,
        width: 108,
        x: 12,
        y: 8,
        toJSON: () => ({}),
      }),
    });
    range.selectNodeContents(textNode!);
    const browserSelection = window.getSelection();
    browserSelection?.removeAllRanges();
    browserSelection?.addRange(range);
    await act(async () => {
      paragraph?.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
      await Promise.resolve();
    });

    const addButton = [...container.querySelectorAll("button")].find(
      (button) => button.textContent === "添加到对话",
    );
    expect(addButton).toBeDefined();
    await act(async () => addButton?.click());
    expect(onAddTextSelection).toHaveBeenCalledWith(expect.objectContaining({
      startLine: 3,
      endLine: 3,
      selectedText: "First paragraph.",
    }));
  });

  it("switches a rendered selection to source and opens the existing line comment editor", async () => {
    const onCreateComment = vi.fn();
    await act(async () => {
      root.render(
        <MarkdownPreview
          preview={{ ...preview, content: "# Title\n\nFirst line\nSecond line\n" }}
          frameProps={frameProps}
          onCreateComment={onCreateComment}
        />,
      );
    });
    const textNode = container.querySelector("p")?.firstChild;
    const range = document.createRange();
    Object.defineProperty(range, "getBoundingClientRect", {
      configurable: true,
      value: () => ({
        bottom: 24, height: 16, left: 12, right: 120, top: 8, width: 108, x: 12, y: 8,
        toJSON: () => ({}),
      }),
    });
    range.selectNodeContents(textNode!);
    window.getSelection()?.removeAllRanges();
    window.getSelection()?.addRange(range);
    await act(async () => {
      container.querySelector("p")?.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
      await Promise.resolve();
    });
    await act(async () => {
      [...container.querySelectorAll("button")]
        .find((button) => button.textContent === "评论")
        ?.click();
    });
    await settleUntil(
      () => container.querySelector(".project-file-line-comment-editor textarea") !== null,
    );

    expect(container.querySelector(".cm-editor")).not.toBeNull();
    expect(container.querySelectorAll(".project-file-line-comment-selection")).toHaveLength(2);
    const textarea = container.querySelector<HTMLTextAreaElement>(
      ".project-file-line-comment-editor textarea",
    );
    expect(textarea?.getAttribute("aria-label")).toBe("第 3–4 行 评论内容");

    textarea?.focus();
    const typingKey = new KeyboardEvent("keydown", {
      key: "修",
      bubbles: true,
      cancelable: true,
    });
    textarea?.dispatchEvent(typingKey);
    expect(typingKey.defaultPrevented).toBe(false);
    await act(async () => {
      if (textarea) textarea.value = "请修改这两行";
      textarea?.dispatchEvent(new InputEvent("input", { bubbles: true }));
    });
    const submit = [...container.querySelectorAll<HTMLButtonElement>(
      ".project-file-line-comment-editor button",
    )].find((button) => button.textContent === "评论");
    expect(submit?.disabled).toBe(false);
    await act(async () => submit?.click());
    expect(onCreateComment).toHaveBeenCalledWith(
      {
        startLine: 3,
        endLine: 4,
        selectedText: "First line\nSecond line",
      },
      "请修改这两行",
    );
  });


});
