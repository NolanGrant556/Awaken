// @vitest-environment jsdom

import { act, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import { AwakenComposerBar } from "@awaken/design-system";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("AwakenComposerBar unified token editing", () => {
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
  });

  afterEach(() => {
    if (root) {
      act(() => {
        root?.unmount();
      });
      root = undefined;
    }
    host.remove();
    delete (
      globalThis as typeof globalThis & {
        IS_REACT_ACT_ENVIRONMENT?: boolean;
      }
    ).IS_REACT_ACT_ENVIRONMENT;
  });

  const defaultToken = {
    id: "skill:documents:user",
    type: "skill",
    label: "Documents",
    leadingText: "你使用这个技能 ",
  };

  function renderComposer(
    overrides: Partial<ComponentProps<typeof AwakenComposerBar>> = {},
  ) {
    const props: ComponentProps<typeof AwakenComposerBar> = {
      value: "",
      onChange: vi.fn(),
      onSubmit: vi.fn(),
      onStructuredInputChange: vi.fn(),
      tokens: [defaultToken],
      ...overrides,
    };
    root ??= createRoot(host);
    act(() => {
      root?.render(<AwakenComposerBar {...props} />);
    });
    const editor = host.querySelector<HTMLDivElement>(
      '.composer-bar-inline-editor[contenteditable="plaintext-only"]',
    );
    expect(editor).not.toBeNull();
    return {
      editor: editor!,
      props,
      rerender(nextOverrides: Partial<ComponentProps<typeof AwakenComposerBar>>) {
        const nextProps = { ...props, ...nextOverrides };
        act(() => {
          root?.render(<AwakenComposerBar {...nextProps} />);
        });
        return nextProps;
      },
    };
  }

  function textNodes(editor: HTMLElement): Text[] {
    return Array.from(editor.childNodes).filter(
      (node): node is Text => node.nodeType === Node.TEXT_NODE,
    );
  }

  function lastTextNode(editor: HTMLElement): Text {
    const nodes = textNodes(editor);
    return nodes[nodes.length - 1]!;
  }

  function collapseSelection(node: Node, offset: number): void {
    const range = document.createRange();
    range.setStart(node, offset);
    range.collapse(true);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  }

  function selectRange(
    startNode: Node,
    startOffset: number,
    endNode: Node,
    endOffset: number,
  ): void {
    const range = document.createRange();
    range.setStart(startNode, startOffset);
    range.setEnd(endNode, endOffset);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
  }

  function dispatchInput(editor: HTMLElement): void {
    act(() => {
      editor.dispatchEvent(new InputEvent("input", { bubbles: true }));
    });
  }

  it("uses one plaintext editing host with atomic token children", () => {
    const { editor } = renderComposer({ value: "写" });
    const token = editor.querySelector<HTMLElement>(
      '[data-composer-token-id="skill:documents:user"]',
    );

    expect(host.querySelectorAll('[contenteditable="plaintext-only"]')).toHaveLength(1);
    expect(token?.getAttribute("contenteditable")).toBe("false");
    expect(token?.textContent).toContain("Documents");
    expect(editor.textContent).toContain("你使用这个技能 ");
    expect(textNodes(editor).map((node) => node.data)).toEqual([
      "你使用这个技能 ",
      "写",
    ]);
    expect(host.querySelector("textarea")).toBeNull();
  });

  it("maps edits before and after the token back to structured segments", () => {
    const onChange = vi.fn();
    const onStructuredInputChange = vi.fn();
    const { editor } = renderComposer({
      value: "原尾文",
      onChange,
      onStructuredInputChange,
    });
    const [leadingText, trailingText] = textNodes(editor);
    expect(leadingText).toBeDefined();
    expect(trailingText).toBeDefined();

    leadingText!.data = "已修改前文 ";
    trailingText!.data = "已修改后文";
    dispatchInput(editor);

    expect(onChange).toHaveBeenLastCalledWith("已修改后文");
    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [
        {
          id: "skill:documents:user",
          leadingText: "已修改前文 ",
        },
      ],
      value: "已修改后文",
    });
  });

  it("keeps multiple token order and every text segment stable", () => {
    const onStructuredInputChange = vi.fn();
    const tokens = [
      { ...defaultToken, leadingText: "第一段 " },
      {
        id: "skill:pptx:user",
        type: "skill",
        label: "pptx",
        leadingText: " 中间段 ",
      },
    ];
    const { editor } = renderComposer({
      value: "最后一段",
      tokens,
      onStructuredInputChange,
    });

    expect(
      Array.from(editor.querySelectorAll<HTMLElement>("[data-composer-token-id]")).map(
        (token) => token.dataset.composerTokenId,
      ),
    ).toEqual(["skill:documents:user", "skill:pptx:user"]);
    const [, middleText] = textNodes(editor);
    middleText!.data = " 修改后的中间段 ";
    dispatchInput(editor);

    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [
        { id: "skill:documents:user", leadingText: "第一段 " },
        { id: "skill:pptx:user", leadingText: " 修改后的中间段 " },
      ],
      value: "最后一段",
    });
  });

  it("removes an adjacent token atomically with Backspace and restores its text", () => {
    const onChange = vi.fn();
    const onStructuredInputChange = vi.fn();
    const { editor } = renderComposer({
      value: "后文",
      onChange,
      onStructuredInputChange,
    });
    const trailingText = lastTextNode(editor);
    editor.focus();
    collapseSelection(trailingText, 0);

    act(() => {
      editor.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Backspace",
          bubbles: true,
          cancelable: true,
        }),
      );
    });

    expect(editor.querySelector("[data-composer-token-id]")).toBeNull();
    expect(onChange).toHaveBeenLastCalledWith("你使用这个技能 后文");
    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [],
      value: "你使用这个技能 后文",
    });
  });

  it("removes an adjacent token atomically with forward Delete", () => {
    const onStructuredInputChange = vi.fn();
    const { editor } = renderComposer({
      value: "后文",
      onStructuredInputChange,
    });
    const leadingText = textNodes(editor)[0]!;
    editor.focus();
    collapseSelection(leadingText, leadingText.length);

    act(() => {
      editor.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Delete",
          bubbles: true,
          cancelable: true,
        }),
      );
    });

    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [],
      value: "你使用这个技能 后文",
    });
  });

  it("deletes a cross-token selection without leaking or duplicating the label", () => {
    const onStructuredInputChange = vi.fn();
    const { editor } = renderComposer({
      value: "后文",
      onStructuredInputChange,
    });
    const [leadingText, trailingText] = textNodes(editor);
    editor.focus();
    selectRange(leadingText!, 2, trailingText!, 1);

    act(() => {
      editor.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Backspace",
          bubbles: true,
          cancelable: true,
        }),
      );
    });

    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [],
      value: "你使文",
    });
    expect(editor.textContent).toBe("你使文");
  });

  it("normalizes Chromium block line breaks back to plaintext segments", () => {
    const onStructuredInputChange = vi.fn();
    const { editor } = renderComposer({ onStructuredInputChange });
    const token = editor.querySelector<HTMLElement>("[data-composer-token-id]")!;
    const firstLine = document.createElement("div");
    firstLine.textContent = "第一行";
    const secondLine = document.createElement("div");
    secondLine.textContent = "第二行";
    editor.replaceChildren(
      document.createTextNode("前文 "),
      token,
      firstLine,
      secondLine,
    );
    dispatchInput(editor);

    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [
        { id: "skill:documents:user", leadingText: "前文 " },
      ],
      value: "\n第一行\n第二行",
    });
  });

  it("copies only plaintext and excludes token labels", () => {
    const { editor } = renderComposer({ value: "后文" });
    const selection = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(editor);
    selection?.removeAllRanges();
    selection?.addRange(range);
    const setData = vi.fn();
    const copyEvent = new Event("copy", { bubbles: true, cancelable: true });
    Object.defineProperty(copyEvent, "clipboardData", {
      value: { setData },
    });

    act(() => {
      editor.dispatchEvent(copyEvent);
    });

    expect(setData).toHaveBeenCalledWith(
      "text/plain",
      "你使用这个技能 后文",
    );
  });

  it("does not reconcile controlled children while IME composition is active", async () => {
    const onSubmit = vi.fn();
    const onStructuredInputChange = vi.fn();
    const { editor, rerender } = renderComposer({
      onSubmit,
      onStructuredInputChange,
    });
    const trailingText = lastTextNode(editor);

    act(() => {
      editor.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
      trailingText.data = "写";
      editor.dispatchEvent(
        new CompositionEvent("compositionupdate", {
          bubbles: true,
          data: "写",
        }),
      );
      editor.dispatchEvent(new InputEvent("input", { bubbles: true }));
    });
    rerender({ value: "外部值" });
    expect(lastTextNode(editor).data).toBe("写");
    expect(onStructuredInputChange).not.toHaveBeenCalled();

    act(() => {
      editor.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Enter",
          bubbles: true,
          cancelable: true,
        }),
      );
    });
    expect(onSubmit).not.toHaveBeenCalled();

    await act(async () => {
      editor.dispatchEvent(
        new CompositionEvent("compositionend", { bubbles: true, data: "写" }),
      );
      await Promise.resolve();
    });
    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [
        {
          id: "skill:documents:user",
          leadingText: "你使用这个技能 ",
        },
      ],
      value: "写",
    });
  });

  it("preserves a logical caret when an external draft restore rebuilds the DOM", () => {
    const { editor, rerender } = renderComposer({ value: "旧尾文" });
    const leadingText = textNodes(editor)[0]!;
    editor.focus();
    collapseSelection(leadingText, 2);

    rerender({ value: "恢复后的尾文" });

    const selection = window.getSelection();
    expect(selection?.isCollapsed).toBe(true);
    expect(selection?.anchorNode).toBe(textNodes(editor)[0]);
    expect(selection?.anchorOffset).toBe(2);
    expect(lastTextNode(editor).data).toBe("恢复后的尾文");
  });

  it("restores externally supplied token identities and segment text", () => {
    const { editor, rerender } = renderComposer({ value: "旧尾文" });
    rerender({
      value: "新尾文",
      tokens: [
        {
          id: "skill:docx:user",
          type: "skill",
          label: "docx",
          leadingText: "新前文 ",
        },
      ],
    });

    expect(editor.querySelector("[data-composer-token-id]")?.textContent).toContain(
      "docx",
    );
    expect(textNodes(editor).map((node) => node.data)).toEqual([
      "新前文 ",
      "新尾文",
    ]);
  });

  it("keeps the trailing segment editable after a second slash token is selected", () => {
    const onStructuredInputChange = vi.fn();
    const { editor, rerender } = renderComposer({
      value: "第二个前文 /docx",
      onStructuredInputChange,
    });
    rerender({
      value: "",
      tokens: [
        defaultToken,
        {
          id: "skill:docx:user",
          type: "skill",
          label: "docx",
          leadingText: "第二个前文 ",
        },
      ],
    });
    const trailingText = lastTextNode(editor);
    trailingText.data = "继续写";
    dispatchInput(editor);

    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [
        {
          id: "skill:documents:user",
          leadingText: "你使用这个技能 ",
        },
        { id: "skill:docx:user", leadingText: "第二个前文 " },
      ],
      value: "继续写",
    });
  });

  it("submits on Enter but preserves Shift+Enter", () => {
    const onSubmit = vi.fn();
    const { editor } = renderComposer({ onSubmit });

    act(() => {
      editor.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Enter",
          shiftKey: true,
          bubbles: true,
          cancelable: true,
        }),
      );
    });
    expect(onSubmit).not.toHaveBeenCalled();

    act(() => {
      editor.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: "Enter",
          bubbles: true,
          cancelable: true,
        }),
      );
    });
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});
