// @vitest-environment jsdom

import { act, type ComponentProps } from "react";
import { createRoot, type Root } from "react-dom/client";
import {
  AwakenComposerBar,
  AwakenUserPromptBlock,
  plainTextFromPromptClipboardRoot,
} from "@awaken/design-system";
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
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 768,
    });
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

  function renderComposer(overrides: Partial<ComponentProps<typeof AwakenComposerBar>> = {}) {
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
    expect(token?.querySelector(".aw-icon--book-open")).not.toBeNull();
    expect(editor.textContent).toContain("你使用这个技能 ");
    expect(textNodes(editor).map((node) => node.data)).toEqual(["你使用这个技能 ", "写"]);
    expect(host.querySelector("textarea")).toBeNull();
  });

  it("highlights the complete atomic token, including its icon, when the selection crosses it", () => {
    const { editor } = renderComposer({ value: "写" });
    const token = editor.querySelector<HTMLElement>(
      '[data-composer-token-id="skill:documents:user"]',
    );
    expect(token).not.toBeNull();

    editor.focus();
    const range = document.createRange();
    range.selectNodeContents(editor);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    act(() => document.dispatchEvent(new Event("selectionchange")));

    expect(selection?.toString()).toContain("Documents");
    expect(token?.hasAttribute("data-composer-token-selected")).toBe(true);
    expect(token?.querySelector(".composer-bar-token-icon")).not.toBeNull();

    const trailingText = lastTextNode(editor);
    collapseSelection(trailingText, trailingText.length);
    act(() => document.dispatchEvent(new Event("selectionchange")));
    expect(token?.hasAttribute("data-composer-token-selected")).toBe(false);
  });

  it("renders capability tokens as an atomic Computer Use chip with a monitor icon", () => {
    const { editor } = renderComposer({
      tokens: [
        {
          id: "capability:computer-use",
          type: "capability",
          label: "Computer Use",
        },
      ],
    });
    const token = editor.querySelector<HTMLElement>(
      '[data-composer-token-id="capability:computer-use"]',
    );

    expect(token?.textContent).toBe("Computer Use");
    const icon = token?.querySelector("svg");
    expect(icon?.classList.contains("aw-icon--display-pointer")).toBe(true);
    expect(icon?.getAttribute("width")).toBe("16");
    expect(icon?.getAttribute("height")).toBe("16");
    expect(token?.querySelectorAll("path").length).toBeGreaterThan(0);
  });

  it("renders Deep Research workflow tokens with the same search icon as the mention menu", () => {
    const { editor } = renderComposer({
      tokens: [
        {
          id: "workflow:deep-research",
          type: "workflow",
          label: "深度研究",
        },
      ],
    });
    const icon = editor.querySelector(
      '[data-composer-token-id="workflow:deep-research"] svg',
    );

    expect(icon?.classList.contains("aw-icon--magnifying-glass")).toBe(true);
    expect(icon?.classList.contains("aw-icon--package")).toBe(false);
    expect(icon?.getAttribute("width")).toBe("16");
    expect(icon?.getAttribute("height")).toBe("16");
  });

  it("uses the same readable line icon size for Plugin tokens", () => {
    const { editor } = renderComposer({
      tokens: [{ id: "plugin:business-review:digest", type: "plugin", label: "Business Review" }],
    });
    const icon = editor.querySelector('[data-composer-token-id="plugin:business-review:digest"] svg');

    expect(icon?.classList.contains("aw-icon--puzzle")).toBe(true);
    expect(icon?.getAttribute("width")).toBe("16");
    expect(icon?.getAttribute("height")).toBe("16");
  });

  it("uses the official Outlook mark instead of the generic Plugin icon", () => {
    const { editor } = renderComposer({
      tokens: [
        {
          id: "plugin:awaken-outlook:digest",
          type: "plugin",
          label: "Outlook 邮箱",
          brandIcon: "outlook",
        },
      ],
    });
    const token = editor.querySelector<HTMLElement>(
      '[data-composer-token-id="plugin:awaken-outlook:digest"]',
    );

    expect(token?.querySelector(".composer-bar-token-brand-icon")).not.toBeNull();
    expect(token?.querySelector(".aw-icon--puzzle")).toBeNull();
  });

  it("groups mention actions under the visible built-in and plugin section labels", () => {
    renderComposer({
      value: "@",
      slashPalette: {
        open: true,
        activeActionId: "workflow:deep-research",
        actions: [
          {
            id: "workflow:deep-research",
            type: "workflow",
            label: "深度研究",
            sectionLabel: "内置",
          },
          {
            id: "capability:computer-use",
            type: "capability",
            label: "Computer Use",
            sectionLabel: "内置",
          },
          {
            id: "plugin:documents:digest",
            type: "plugin",
            label: "Documents",
            sectionLabel: "插件",
          },
        ],
      },
    });

    const sections = Array.from(
      host.querySelectorAll<HTMLElement>(".composer-bar-slash-section"),
    );
    expect(sections.map((section) => section.getAttribute("aria-label"))).toEqual([
      "内置",
      "插件",
    ]);
    expect(
      sections[0]?.querySelectorAll(".composer-bar-slash-option"),
    ).toHaveLength(2);
    expect(
      sections[1]?.querySelectorAll(".composer-bar-slash-option"),
    ).toHaveLength(1);
    expect(
      Array.from(host.querySelectorAll(".composer-bar-slash-section-label")).map(
        (label) => label.textContent,
      ),
    ).toEqual(["内置", "插件"]);
  });

  it("keeps the same token geometry in home, conversation and embedded composers", () => {
    const { editor, rerender } = renderComposer({ variant: "center" });
    expect(editor.closest(".composer-bar")?.classList).toContain("composer-bar-center");
    expect(editor.querySelector("svg")?.getAttribute("width")).toBe("16");

    rerender({ variant: "dock" });
    expect(host.querySelector(".composer-bar")?.classList).toContain("composer-bar-dock");
    expect(host.querySelector(".composer-bar-token svg")?.getAttribute("width")).toBe("16");

    rerender({ variant: "embedded" });
    expect(host.querySelector(".composer-bar")?.classList).toContain("composer-bar-embedded");
    expect(host.querySelector(".composer-bar-token svg")?.getAttribute("width")).toBe("16");
  });

  it("renders file tokens from the Awaken package icon registry", () => {
    const { editor } = renderComposer({
      tokens: [{ id: "file-token", type: "file", label: "上下文" }],
    });
    const token = editor.querySelector('[data-composer-token-id="file-token"]');
    expect(token?.querySelector("svg")?.classList.contains("aw-icon--package")).toBe(true);
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
    editor.replaceChildren(document.createTextNode("前文 "), token, firstLine, secondLine);
    dispatchInput(editor);

    expect(onStructuredInputChange).toHaveBeenLastCalledWith({
      tokens: [{ id: "skill:documents:user", leadingText: "前文 " }],
      value: "\n第一行\n第二行",
    });
  });

  it("serializes prompt selection without soft-wrap newlines between token and text", () => {
    const root = document.createElement("div");
    const token = document.createElement("span");
    token.className = "execution-prompt-bubble-token";
    const icon = document.createElement("span");
    icon.className = "execution-prompt-bubble-token-icon";
    icon.setAttribute("aria-hidden", "true");
    icon.textContent = "icon";
    const label = document.createElement("span");
    label.className = "execution-prompt-bubble-token-label";
    label.textContent = "2026年上半年经营管理部经营数据可视化图表.pdf";
    token.append(icon, label);
    root.append(token, document.createTextNode("你能看到这个文档吗？"));

    expect(plainTextFromPromptClipboardRoot(root)).toBe(
      "2026年上半年经营管理部经营数据可视化图表.pdf 你能看到这个文档吗？",
    );
  });

  it("highlights the complete prompt token when the selection crosses its label", () => {
    root ??= createRoot(host);
    act(() => {
      root?.render(
        <AwakenUserPromptBlock
          text="？这个是什么来的？"
          tokens={[
            {
              id: "skill:hyperframes-animation:user",
              type: "skill",
              label: "hyperframes-animation",
              leadingText: "你看到这个 skill 吗 ",
            },
          ]}
        />,
      );
    });
    const selectRoot = host.querySelector<HTMLElement>("[data-aw-select-all-root]")!;
    const token = selectRoot.querySelector<HTMLElement>(
      ".execution-prompt-bubble-token",
    )!;
    const range = document.createRange();
    range.selectNodeContents(selectRoot);
    const selection = window.getSelection();
    selection?.removeAllRanges();
    selection?.addRange(range);
    act(() => document.dispatchEvent(new Event("selectionchange")));

    expect(token.hasAttribute("data-prompt-token-selected")).toBe(true);
    expect(token.querySelector(".execution-prompt-bubble-token-icon")).not.toBeNull();

    collapseSelection(selectRoot, selectRoot.childNodes.length);
    act(() => document.dispatchEvent(new Event("selectionchange")));
    expect(token.hasAttribute("data-prompt-token-selected")).toBe(false);
  });

  it("uses the same 16px token icon contract in the submitted message card", () => {
    root ??= createRoot(host);
    act(() => {
      root?.render(
        <AwakenUserPromptBlock
          text="nimen 123"
          tokens={[
            { id: "capability:computer-use", type: "capability", label: "Computer Use" },
            {
              id: "skill:code-search:user",
              type: "skill",
              label: "code-search",
              leadingText: "我们 women ",
            },
            {
              id: "plugin:awaken-outlook:digest",
              type: "plugin",
              label: "Outlook 邮箱",
              leadingText: "2026 ",
              brandIcon: "outlook",
            },
          ]}
        />,
      );
    });

    const icons = Array.from(
      host.querySelectorAll<SVGElement>(".execution-prompt-bubble-token-icon"),
    ).filter((icon) => icon.tagName.toLowerCase() === "svg");
    expect(icons).toHaveLength(2);
    expect(icons.every((icon) => icon.getAttribute("width") === "16")).toBe(true);
    expect(icons[0]?.classList).toContain("aw-icon--display-pointer");
    expect(icons[1]?.classList).toContain("aw-icon--book-open");
    expect(host.querySelector(".execution-prompt-bubble-token-brand-icon")).not.toBeNull();
    expect(
      host.querySelector(".execution-prompt-bubble-token-plugin .aw-icon--puzzle"),
    ).toBeNull();
  });

  it("copies visible plaintext with token labels and no soft-wrap breaks", () => {
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
      "你使用这个技能 Documents 后文",
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
      editor.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "写" }));
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

    expect(editor.querySelector("[data-composer-token-id]")?.textContent).toContain("docx");
    expect(textNodes(editor).map((node) => node.data)).toEqual(["新前文 ", "新尾文"]);
  });

  it("renders project paths as ordinary atomic name tokens and focuses the requested logical offset", () => {
    const onInputSelectionChange = vi.fn();
    const { editor } = renderComposer({
      value: "后文",
      focusRequest: 1,
      focusOffset: 4,
      onInputSelectionChange,
      tokens: [
        {
          id: "project-path:brief",
          type: "file",
          label: "brief.md",
          leadingText: "前文",
        },
      ],
    });

    const token = editor.querySelector<HTMLElement>(
      '[data-composer-token-id="project-path:brief"]',
    );
    expect(token?.textContent).toContain("brief.md");
    expect(editor.textContent).not.toContain("/workspace/docs/brief.md");
    expect(token?.getAttribute("contenteditable")).toBe("false");
    expect(onInputSelectionChange).toHaveBeenCalledWith({
      anchor: 4,
      focus: 4,
    });
  });

  it("does not reposition the inline caret when only the selection callback identity changes", () => {
    const replacementSelectionChange = vi.fn();
    const { editor, rerender } = renderComposer({
      value: "后文",
      focusRequest: 1,
      focusOffset: 1,
      onInputSelectionChange: vi.fn(),
    });
    const trailingText = lastTextNode(editor);
    editor.focus();
    collapseSelection(trailingText, trailingText.data.length);

    rerender({ onInputSelectionChange: replacementSelectionChange });

    const selection = window.getSelection();
    expect(selection?.anchorNode).toBe(trailingText);
    expect(selection?.anchorOffset).toBe(trailingText.data.length);
    expect(replacementSelectionChange).not.toHaveBeenCalled();
  });

  it("does not reset a textarea caret on controlled rerenders with the same focus request", () => {
    const firstProps: ComponentProps<typeof AwakenComposerBar> = {
      value: "P",
      onChange: vi.fn(),
      onSubmit: vi.fn(),
      tokens: [],
      focusRequest: 1,
      focusOffset: 1,
      onInputSelectionChange: vi.fn(),
    };
    root ??= createRoot(host);
    act(() => root?.render(<AwakenComposerBar {...firstProps} />));
    const textarea = host.querySelector<HTMLTextAreaElement>("textarea");
    expect(textarea).not.toBeNull();
    textarea!.focus();
    textarea!.value = "Pl";
    textarea!.setSelectionRange(2, 2);

    act(() =>
      root?.render(
        <AwakenComposerBar {...firstProps} value="Pl" onInputSelectionChange={vi.fn()} />,
      ),
    );

    expect(textarea?.selectionStart).toBe(2);
    expect(textarea?.selectionEnd).toBe(2);
  });

  it("reports the last textarea caret when native focus leaves for a context menu", () => {
    const onInputSelectionChange = vi.fn();
    const props: ComponentProps<typeof AwakenComposerBar> = {
      value: "Please analyze and summarize",
      onChange: vi.fn(),
      onSubmit: vi.fn(),
      tokens: [],
      onInputSelectionChange,
    };
    root ??= createRoot(host);
    act(() => root?.render(<AwakenComposerBar {...props} />));
    const textarea = host.querySelector<HTMLTextAreaElement>("textarea")!;
    textarea.focus();
    textarea.setSelectionRange(14, 14);

    act(() => textarea.blur());

    expect(onInputSelectionChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        anchor: 14,
        focus: 14,
        caretRect: expect.objectContaining({
          left: expect.any(Number),
          top: expect.any(Number),
          bottom: expect.any(Number),
        }),
      }),
    );
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

  it("renders the center project shelf and forwards selection and creation", () => {
    const onSelectAwaken = vi.fn();
    const onSelectErp = vi.fn();
    const onClearProject = vi.fn();
    const onCreateProject = vi.fn();
    renderComposer({
      variant: "center",
      projectPicker: {
        placeholder: "选择项目",
        options: [
          {
            id: "awaken",
            label: "Awaken",
            selected: true,
            onSelect: onSelectAwaken,
          },
          { id: "erp", label: "ERP 项目", onSelect: onSelectErp },
        ],
        clearAction: {
          label: "取消选择项目",
          onClick: onClearProject,
        },
        createAction: {
          label: "创建项目",
          onClick: onCreateProject,
        },
      },
    });

    const trigger = host.querySelector<HTMLButtonElement>(
      '.composer-bar-project-trigger[aria-haspopup="menu"]',
    );
    expect(trigger?.textContent).toContain("Awaken");
    expect(trigger?.querySelector("button")).toBeNull();

    const clearProject = host.querySelector<HTMLButtonElement>(".composer-bar-project-clear");
    expect(clearProject?.getAttribute("aria-label")).toBe("取消选择项目：Awaken");
    act(() => {
      clearProject?.click();
    });
    expect(onClearProject).toHaveBeenCalledTimes(1);
    expect(document.querySelector(".composer-bar-project-menu")).toBeNull();

    act(() => {
      trigger?.click();
    });
    const erpOption = Array.from(
      document.querySelectorAll<HTMLButtonElement>(
        '.composer-bar-project-option[role="menuitemradio"]',
      ),
    ).find((option) => option.textContent?.includes("ERP 项目"));
    expect(erpOption).toBeDefined();
    act(() => {
      erpOption?.click();
    });
    expect(onSelectErp).toHaveBeenCalledTimes(1);
    expect(onSelectAwaken).not.toHaveBeenCalled();

    act(() => {
      trigger?.click();
    });
    const createAction = Array.from(
      document.querySelectorAll<HTMLButtonElement>('.composer-bar-project-create[role="menuitem"]'),
    ).find((button) => button.textContent?.includes("创建项目"));
    expect(createAction?.textContent).toBe("创建项目");
    act(() => {
      createAction?.click();
    });
    expect(onCreateProject).toHaveBeenCalledTimes(1);
  });

  it("renders the global conversation scope as an iconless unselected-project state", () => {
    renderComposer({
      variant: "center",
      projectPicker: {
        label: "选择项目",
        placeholder: "选择项目",
        options: [
          {
            id: "conversation",
            label: "选择项目",
            icon: null,
            selected: true,
          },
          { id: "erp", label: "ERP 项目" },
        ],
      },
    });

    const trigger = host.querySelector<HTMLButtonElement>(
      '.composer-bar-project-trigger[aria-haspopup="menu"]',
    );
    expect(trigger?.getAttribute("aria-label")).toBe("选择项目，当前：选择项目");
    expect(trigger?.firstElementChild?.classList).toContain("composer-bar-project-trigger-label");
    expect(trigger?.classList.contains("composer-bar-project-trigger-no-icon")).toBe(true);

    act(() => {
      trigger?.click();
    });
    const options = Array.from(
      document.querySelectorAll<HTMLButtonElement>(
        '.composer-bar-project-option[role="menuitemradio"]',
      ),
    );
    const globalOption = options.find((option) => option.textContent?.includes("选择项目"));
    const projectOption = options.find((option) => option.textContent?.includes("ERP 项目"));
    expect(globalOption?.firstElementChild?.classList).toContain(
      "composer-bar-project-option-label",
    );
    expect(globalOption?.classList.contains("composer-bar-project-option-no-icon")).toBe(true);
    expect(projectOption?.firstElementChild?.tagName).toBe("svg");
  });

  it("keeps the project shelf out of the dock composer", () => {
    renderComposer({
      variant: "dock",
      projectPicker: {
        options: [
          {
            id: "awaken",
            label: "Awaken",
            selected: true,
          },
        ],
      },
    });

    expect(host.querySelector(".composer-bar-project-shelf")).toBeNull();
  });

  it("previews at most three plugin icons and selects from the compact full list", () => {
    const onSelectOption = vi.fn();
    const { rerender } = renderComposer({
      variant: "center",
      pluginPicker: {
        options: [
          { id: "documents", label: "Documents" },
          { id: "pdf", label: "PDF" },
          { id: "spreadsheets", label: "Spreadsheets" },
          { id: "sites", label: "Sites" },
        ],
        onSelectOption,
      },
    });

    const trigger = host.querySelector<HTMLButtonElement>(".composer-bar-plugin-trigger");
    expect(trigger?.getAttribute("aria-label")).toBe("插件，4 个可用");
    expect(trigger?.querySelectorAll(".composer-bar-plugin-badge")).toHaveLength(3);
    expect(trigger?.querySelectorAll(".aw-icon--puzzle")).toHaveLength(3);
    expect(
      Array.from(trigger?.querySelectorAll<SVGElement>(".aw-icon--puzzle") ?? []).every(
        (icon) => icon.getAttribute("width") === "12",
      ),
    ).toBe(true);

    act(() => {
      trigger?.click();
    });
    expect(document.querySelectorAll(".composer-bar-plugin-option")).toHaveLength(4);
    expect(document.querySelector(".composer-bar-plugin-search")).toBeNull();
    expect(document.querySelectorAll(".composer-bar-plugin-option .aw-icon--puzzle")).toHaveLength(
      4,
    );
    expect(
      Array.from(
        document.querySelectorAll<SVGElement>(".composer-bar-plugin-option .aw-icon--puzzle"),
      ).every((icon) => icon.getAttribute("width") === "12"),
    ).toBe(true);
    const sitesOption = Array.from(
      document.querySelectorAll<HTMLButtonElement>(".composer-bar-plugin-option"),
    ).find((option) => option.textContent?.includes("Sites"));
    act(() => {
      sitesOption?.click();
    });
    expect(onSelectOption).toHaveBeenCalledWith("sites");
    expect(document.querySelector(".composer-bar-plugin-menu")).toBeNull();

    rerender({ variant: "embedded" });
    expect(host.querySelector(".composer-bar-plugin-trigger")).toBeNull();
  });

  it("keeps the plugin entry visible and explains the empty installed state", () => {
    renderComposer({
      pluginPicker: {
        options: [],
      },
    });

    const trigger = host.querySelector<HTMLButtonElement>(".composer-bar-plugin-trigger");
    expect(trigger?.getAttribute("aria-label")).toBe("插件，暂无可用插件");
    expect(trigger?.querySelectorAll(".aw-icon--puzzle")).toHaveLength(1);
    act(() => {
      trigger?.click();
    });
    expect(document.querySelector(".composer-bar-plugin-search")).toBeNull();
    expect(document.querySelector(".composer-bar-plugin-empty")?.textContent).toBe(
      "还没有安装插件",
    );
  });

  it("renders the selected model logo in the composer trigger", () => {
    renderComposer({
      metaItems: [
        {
          id: "model",
          label: "kimi-for-coding",
          icon: <span data-testid="selected-model-logo">K</span>,
          menuItems: [{ id: "profile-kimi", label: "kimi-for-coding", selected: true }],
        },
      ],
    });

    const trigger = host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger");
    expect(trigger?.querySelector(".composer-bar-model-trigger-icon")).not.toBeNull();
    expect(trigger?.querySelector('[data-testid="selected-model-logo"]')).not.toBeNull();
    const chevron = trigger?.querySelector<HTMLElement>(
      ".composer-bar-model-trigger-chevron",
    );
    expect(chevron?.getAttribute("aria-hidden")).toBe("true");
    expect(chevron?.querySelector(".aw-icon--chevron-down")?.getAttribute("width")).toBe(
      "14",
    );
    expect(trigger?.getAttribute("aria-label")).toBe("kimi-for-coding");
    expect(trigger?.title).toBe("kimi-for-coding");
    expect(trigger?.getAttribute("aria-expanded")).toBe("false");
  });

  it("stretches the model trigger itself in both center and dock composers", () => {
    const metaItems = [
      {
        id: "model",
        label: "k3 · high",
        menuItems: [{ id: "profile-k3", label: "k3", selected: true }],
      },
    ];
    const { rerender } = renderComposer({ variant: "center", metaItems });
    const trigger = host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger")!;
    const bounds = vi.spyOn(trigger, "getBoundingClientRect").mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 136,
      bottom: 32,
      width: 136,
      height: 32,
      toJSON: () => ({}),
    });

    act(() => trigger.click());
    expect(trigger.classList).toContain("composer-bar-model-trigger-open");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(trigger.style.getPropertyValue("--composer-model-trigger-collapsed-width")).toBe(
      "136px",
    );
    expect(host.querySelector(".composer-bar-model-menu")?.className).toBe(
      "composer-bar-model-menu",
    );

    act(() => trigger.click());
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    rerender({ variant: "dock" });
    bounds.mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 118,
      bottom: 27,
      width: 118,
      height: 27,
      toJSON: () => ({}),
    });
    act(() => trigger.click());
    expect(host.querySelector(".composer-bar-dock")).not.toBeNull();
    expect(trigger.classList).toContain("composer-bar-model-trigger-open");
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(trigger.style.getPropertyValue("--composer-model-trigger-collapsed-width")).toBe(
      "118px",
    );
  });

  it("flips the model menu with viewport space while preserving each variant preference", () => {
    const metaItems = [
      {
        id: "model",
        label: "GLM-5.2",
        menuItems: Array.from({ length: 7 }, (_, index) => ({
          id: `profile-${index}`,
          label: `模型 ${index + 1}`,
          selected: index === 0,
        })),
      },
    ];
    const { rerender } = renderComposer({ variant: "center", metaItems });
    const modelRoot = host.querySelector<HTMLElement>(".composer-bar-toolbar-model")!;
    const trigger = host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger")!;
    const bounds = vi.spyOn(modelRoot, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 430,
      left: 100,
      top: 430,
      right: 236,
      bottom: 462,
      width: 136,
      height: 32,
      toJSON: () => ({}),
    });
    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 500,
    });

    act(() => trigger.click());
    expect(modelRoot.dataset.menuPlacement).toBe("top");

    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 900,
    });
    act(() => window.dispatchEvent(new Event("resize")));
    expect(modelRoot.dataset.menuPlacement).toBe("bottom");

    act(() => trigger.click());
    rerender({ variant: "dock", metaItems });
    bounds.mockReturnValue({
      x: 100,
      y: 400,
      left: 100,
      top: 400,
      right: 236,
      bottom: 432,
      width: 136,
      height: 32,
      toJSON: () => ({}),
    });
    act(() => trigger.click());
    expect(modelRoot.dataset.menuPlacement).toBe("top");

    bounds.mockReturnValue({
      x: 100,
      y: 20,
      left: 100,
      top: 20,
      right: 236,
      bottom: 52,
      width: 136,
      height: 32,
      toJSON: () => ({}),
    });
    act(() => window.dispatchEvent(new Event("resize")));
    expect(modelRoot.dataset.menuPlacement).toBe("bottom");
  });

  it("opens an effort secondary panel and immediately changes a two-level control", () => {
    const onChange = vi.fn();
    renderComposer({
      metaItems: [
        {
          id: "model",
          label: "GLM-5.2 · high",
          menuItems: [
            {
              id: "profile-glm",
              label: "GLM-5.2",
              icon: <span data-testid="effort-model-logo">GLM</span>,
              selected: true,
              effort: {
                levels: ["high", "max"],
                selectedLevel: "high",
                onChange,
              },
            },
          ],
        },
      ],
    });

    const trigger = host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger");
    act(() => trigger?.click());
    const option = host.querySelector<HTMLButtonElement>(".composer-bar-model-option");
    act(() => option?.click());

    const panel = host.querySelector<HTMLElement>(".composer-bar-model-effort-panel");
    const slider = host.querySelector<HTMLElement>(".composer-bar-model-effort-slider");
    expect(panel?.getAttribute("aria-label")).toContain("GLM-5.2");
    expect(
      panel?.querySelector(".composer-bar-model-effort-icon [data-testid='effort-model-logo']"),
    ).not.toBeNull();
    expect(slider?.getAttribute("role")).toBe("slider");
    expect(slider?.getAttribute("aria-valuemax")).toBe("1");
    expect(slider?.getAttribute("aria-valuetext")).toBe("high");
    expect(slider?.dataset.effort).toBe("high");
    expect(slider?.textContent).not.toContain("Effort");
    expect(slider?.textContent?.trim()).toBe("");
    expect(slider?.querySelector(".composer-bar-model-effort-value")).toBeNull();
    expect(slider?.querySelectorAll(".composer-bar-model-effort-detent")).toHaveLength(1);
    expect(
      Array.from(slider?.querySelectorAll(".composer-bar-model-effort-detent") ?? []).map(
        (detent) => detent.getAttribute("data-level"),
      ),
    ).toEqual(["max"]);
    expect(
      slider
        ?.querySelector<HTMLElement>(".composer-bar-model-effort-detent[data-level='max']")
        ?.style.getPropertyValue("--composer-effort-detent-progress"),
    ).toBe("92%");
    expect(
      Array.from(slider?.querySelectorAll(".composer-bar-model-effort-detent") ?? []).map(
        (detent) => detent.getAttribute("data-reached"),
      ),
    ).toEqual(["false"]);
    const effortFlow = slider?.querySelector(".composer-bar-model-effort-flow");
    expect(effortFlow?.classList).toContain("aw-memory-light-flow--compact");
    expect(effortFlow?.classList).toContain("aw-memory-light-flow--tone-effort");
    expect(slider?.style.getPropertyValue("--composer-effort-selected-progress")).toBe(
      "0%",
    );
    expect(slider?.querySelector(".composer-bar-model-effort-thumb")).not.toBeNull();

    act(() => {
      slider?.dispatchEvent(
        new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("max");
    expect(slider?.dataset.effort).toBe("max");
    expect(slider?.textContent?.trim()).toBe("");
    expect(effortFlow?.classList).toContain("aw-memory-light-flow--tone-danger");
    expect(slider?.style.getPropertyValue("--composer-effort-selected-progress")).toBe(
      "100%",
    );
    expect(
      Array.from(slider?.querySelectorAll(".composer-bar-model-effort-detent") ?? []).map(
        (detent) => detent.getAttribute("data-reached"),
      ),
    ).toEqual(["true"]);
    expect(onChange).toHaveBeenCalledWith("max");

    act(() => {
      slider?.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Home", bubbles: true }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("high");
    expect(effortFlow?.classList).toContain("aw-memory-light-flow--tone-effort");
    expect(slider?.style.getPropertyValue("--composer-effort-selected-progress")).toBe(
      "0%",
    );
    expect(onChange).toHaveBeenLastCalledWith("high");

    act(() => {
      slider?.dispatchEvent(
        new KeyboardEvent("keydown", { key: "End", bubbles: true }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("max");
    expect(effortFlow?.classList).toContain("aw-memory-light-flow--tone-danger");
    expect(slider?.style.getPropertyValue("--composer-effort-selected-progress")).toBe(
      "100%",
    );
    expect(onChange).toHaveBeenLastCalledWith("max");
    expect(host.querySelector(".composer-bar-model-effort-apply")).toBeNull();
    expect(host.querySelector(".composer-bar-model-effort-panel")).not.toBeNull();

    act(() => trigger?.click());
    expect(host.querySelector(".composer-bar-model-effort-flow")).toBeNull();
  });

  it("shows provider-native reasoning labels without changing canonical effort values", () => {
    renderComposer({
      metaItems: [{
        id: "model",
        label: "qwen3.8-max · xhigh",
        menuItems: [{
          id: "profile-qwen",
          label: "qwen3.8-max",
          selected: true,
          effort: {
            levels: ["low", "high", "max"],
            selectedLevel: "max",
            displayLabels: { low: "low", high: "medium", max: "xhigh" },
            onChange: vi.fn(),
          },
        }],
      }],
    });

    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger")?.click());
    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-option")?.click());

    const slider = host.querySelector<HTMLElement>(".composer-bar-model-effort-slider");
    expect(slider?.getAttribute("aria-valuetext")).toBe("xhigh");
    expect(slider?.dataset.effort).toBe("max");
    expect(
      [...host.querySelectorAll(".composer-bar-model-effort-labels > span")].map(
        (label) => label.textContent,
      ),
    ).toEqual(["low", "medium", "xhigh"]);
  });

  it("selects an effort-capable target model before opening its secondary panel", () => {
    const callOrder: string[] = [];
    renderComposer({
      metaItems: [
        {
          id: "model",
          label: "current · high",
          menuItems: [
            {
              id: "profile-target",
              label: "target",
              onSelect: () => callOrder.push("select-target"),
              effort: {
                levels: ["low", "high", "max"],
                selectedLevel: "high",
                onChange: (level) => callOrder.push(`effort-${level}`),
              },
            },
          ],
        },
      ],
    });

    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger")?.click());
    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-option")?.click());

    expect(callOrder).toEqual(["select-target"]);
    expect(host.querySelector(".composer-bar-model-effort-panel")).not.toBeNull();
    expect(
      host.querySelector(".composer-bar-model-effort-slider")?.getAttribute("aria-valuetext"),
    ).toBe("high");
  });

  it("keeps a two-level max detent inside the right-end value zone", () => {
    const onChange = vi.fn();
    renderComposer({
      metaItems: [
        {
          id: "model",
          label: "GLM-5.2 · high",
          menuItems: [
            {
              id: "profile-glm",
              label: "GLM-5.2",
              effort: {
                levels: ["high", "max"],
                selectedLevel: "high",
                onChange,
              },
            },
          ],
        },
      ],
    });

    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger")?.click());
    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-option")?.click());
    const slider = host.querySelector<HTMLElement>(".composer-bar-model-effort-slider");
    vi.spyOn(slider!, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 0,
      left: 100,
      top: 0,
      right: 300,
      bottom: 32,
      width: 200,
      height: 32,
      toJSON: () => ({}),
    });

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointerdown", { bubbles: true, clientX: 240 }),
      );
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 268 }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("high");
    expect(onChange).not.toHaveBeenCalled();

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 284 }),
      );
      slider?.dispatchEvent(
        new MouseEvent("pointerup", { bubbles: true, clientX: 284 }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("max");
    expect(onChange).toHaveBeenCalledOnce();
    expect(onChange).toHaveBeenCalledWith("max");
  });

  it("snaps the effort slider to discrete levels while dragging", () => {
    const onChange = vi.fn();
    renderComposer({
      metaItems: [
        {
          id: "model",
          label: "k3 · low",
          menuItems: [
            {
              id: "profile-k3",
              label: "k3",
              effort: {
                levels: ["low", "high", "max"],
                selectedLevel: "low",
                onChange,
              },
            },
          ],
        },
      ],
    });

    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger")?.click());
    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-option")?.click());
    const slider = host.querySelector<HTMLElement>(".composer-bar-model-effort-slider");
    expect(slider?.querySelectorAll(".composer-bar-model-effort-detent")).toHaveLength(2);
    expect(
      Array.from(slider?.querySelectorAll(".composer-bar-model-effort-detent") ?? []).map(
        (detent) => detent.getAttribute("data-level"),
      ),
    ).toEqual(["high", "max"]);
    expect(
      Array.from(
        slider?.querySelectorAll<HTMLElement>(".composer-bar-model-effort-detent") ?? [],
      ).map((detent) => detent.style.getPropertyValue("--composer-effort-detent-progress")),
    ).toEqual(["50%", "92%"]);
    expect(slider?.style.getPropertyValue("--composer-effort-selected-progress")).toBe(
      "0%",
    );
    vi.spyOn(slider!, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 0,
      left: 100,
      top: 0,
      right: 458,
      bottom: 32,
      width: 358,
      height: 32,
      toJSON: () => ({}),
    });
    const setPointerCapture = vi.fn();
    const releasePointerCapture = vi.fn();
    Object.defineProperties(slider!, {
      setPointerCapture: { configurable: true, value: setPointerCapture },
      hasPointerCapture: { configurable: true, value: () => true },
      releasePointerCapture: { configurable: true, value: releasePointerCapture },
    });

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 455 }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("low");
    expect(slider?.dataset.dragging).toBeUndefined();
    expect(onChange).not.toHaveBeenCalled();

    act(() => {
      const pointerDown = new MouseEvent("pointerdown", { bubbles: true, clientX: 300 });
      Object.defineProperty(pointerDown, "pointerId", { value: 7 });
      slider?.dispatchEvent(pointerDown);
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("high");
    expect(slider?.dataset.dragging).toBe("true");
    expect(slider?.style.getPropertyValue("--composer-effort-drag-progress")).not.toBe("");
    expect(slider?.style.getPropertyValue("--composer-effort-selected-progress")).toBe(
      "50%",
    );
    expect(
      Array.from(slider?.querySelectorAll(".composer-bar-model-effort-detent") ?? []).map(
        (detent) => detent.getAttribute("data-reached"),
      ),
    ).toEqual(["true", "false"]);
    expect(onChange).toHaveBeenLastCalledWith("high");
    expect(setPointerCapture).toHaveBeenCalledWith(7);

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 304 }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("high");
    expect(onChange.mock.calls.map(([level]) => level)).toEqual(["high"]);

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 395 }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("high");
    expect(onChange.mock.calls.map(([level]) => level)).toEqual(["high"]);

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 432 }),
      );
      const pointerUp = new MouseEvent("pointerup", { bubbles: true, clientX: 432 });
      Object.defineProperty(pointerUp, "pointerId", { value: 7 });
      slider?.dispatchEvent(pointerUp);
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("max");
    expect(slider?.dataset.dragging).toBeUndefined();
    expect(slider?.style.getPropertyValue("--composer-effort-drag-progress")).toBe("");
    expect(slider?.style.getPropertyValue("--composer-effort-selected-progress")).toBe(
      "100%",
    );
    expect(
      slider?.querySelector(".composer-bar-model-effort-flow")?.classList,
    ).toContain("aw-memory-light-flow--tone-danger");
    expect(
      Array.from(slider?.querySelectorAll(".composer-bar-model-effort-detent") ?? []).map(
        (detent) => detent.getAttribute("data-reached"),
      ),
    ).toEqual(["true", "true"]);
    expect(onChange.mock.calls.map(([level]) => level)).toEqual(["high", "max"]);
    expect(slider?.textContent?.trim()).toBe("");
    expect(releasePointerCapture).toHaveBeenCalledWith(7);

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 110 }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("max");
    expect(onChange.mock.calls.map(([level]) => level)).toEqual(["high", "max"]);

    act(() => {
      const pointerDown = new MouseEvent("pointerdown", {
        bubbles: true,
        clientX: 300,
      });
      Object.defineProperty(pointerDown, "pointerId", { value: 8 });
      slider?.dispatchEvent(pointerDown);
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 110 }),
      );
      const pointerCancel = new MouseEvent("pointercancel", { bubbles: true });
      Object.defineProperty(pointerCancel, "pointerId", { value: 8 });
      slider?.dispatchEvent(pointerCancel);
    });
    expect(slider?.dataset.dragging).toBeUndefined();
    expect(slider?.getAttribute("aria-valuetext")).toBe("low");
    expect(releasePointerCapture).toHaveBeenCalledWith(8);

    act(() => {
      slider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 455 }),
      );
    });
    expect(slider?.getAttribute("aria-valuetext")).toBe("low");
    expect(onChange.mock.calls.map(([level]) => level)).toEqual([
      "high",
      "max",
      "high",
      "low",
    ]);
  });

  it("clears an interrupted effort drag before the secondary panel reopens", () => {
    const onChange = vi.fn();
    renderComposer({
      metaItems: [
        {
          id: "model",
          label: "k3 · low",
          menuItems: [
            {
              id: "profile-k3",
              label: "k3",
              effort: {
                levels: ["low", "high", "max"],
                selectedLevel: "low",
                onChange,
              },
            },
          ],
        },
      ],
    });

    const trigger = host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger");
    act(() => trigger?.click());
    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-option")?.click());
    const firstSlider = host.querySelector<HTMLElement>(".composer-bar-model-effort-slider");
    vi.spyOn(firstSlider!, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 0,
      left: 100,
      top: 0,
      right: 458,
      bottom: 32,
      width: 358,
      height: 32,
      toJSON: () => ({}),
    });

    act(() => {
      firstSlider?.dispatchEvent(
        new MouseEvent("pointerdown", { bubbles: true, clientX: 300 }),
      );
    });
    expect(firstSlider?.dataset.dragging).toBe("true");
    expect(onChange).toHaveBeenCalledWith("high");

    act(() => trigger?.click());
    expect(host.querySelector(".composer-bar-model-effort-slider")).toBeNull();
    act(() => trigger?.click());
    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-option")?.click());
    const reopenedSlider = host.querySelector<HTMLElement>(".composer-bar-model-effort-slider");
    vi.spyOn(reopenedSlider!, "getBoundingClientRect").mockReturnValue({
      x: 100,
      y: 0,
      left: 100,
      top: 0,
      right: 458,
      bottom: 32,
      width: 358,
      height: 32,
      toJSON: () => ({}),
    });

    act(() => {
      reopenedSlider?.dispatchEvent(
        new MouseEvent("pointermove", { bubbles: true, clientX: 395 }),
      );
    });
    expect(reopenedSlider?.getAttribute("aria-valuetext")).toBe("low");
    expect(reopenedSlider?.dataset.dragging).toBeUndefined();
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("selects a model without rendering an effort placeholder", () => {
    const onSelect = vi.fn();
    renderComposer({
      metaItems: [
        {
          id: "model",
          label: "MiniMax-M3",
          menuItems: [{ id: "profile-minimax", label: "MiniMax-M3", onSelect }],
        },
      ],
    });

    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-trigger")?.click());
    act(() => host.querySelector<HTMLButtonElement>(".composer-bar-model-option")?.click());
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(host.querySelector(".composer-bar-model-effort-panel")).toBeNull();
  });

  it("renders selected file text as a removable context summary", () => {
    const onRemoveFileComment = vi.fn();
    renderComposer({
      fileComments: [{
        id: "selection-1",
        kind: "selection",
        displayName: "README.md",
        relativePath: "README.md",
        startLine: 2,
        endLine: 3,
        selectedText: "First\nSecond",
        comment: "",
        validationState: "ready",
      }],
      onRemoveFileComment,
    });

    expect(host.textContent).toContain("1 个已选文本片段");
    const clear = host.querySelector<HTMLButtonElement>(
      '[aria-label="移除全部已选文本片段"]',
    );
    expect(clear).not.toBeNull();
    act(() => clear?.click());
    expect(onRemoveFileComment).toHaveBeenCalledWith("selection-1");
  });

  it("portals file context details above the composer stacking context", () => {
    renderComposer({
      fileComments: [{
        id: "selection-1",
        kind: "selection",
        displayName: "README.md",
        relativePath: "README.md",
        startLine: 2,
        endLine: 3,
        selectedText: "First\nSecond",
        comment: "",
        validationState: "ready",
      }],
    });

    act(() => {
      host.querySelector<HTMLButtonElement>(
        '[aria-label="1 个已选文本片段"]',
      )?.click();
    });

    expect(host.querySelector(".composer-file-comments-popover")).toBeNull();
    const popover = document.body.querySelector<HTMLElement>(
      '.composer-file-comments-popover[aria-label="已选文本片段详情"]',
    );
    expect(popover).not.toBeNull();
    expect(popover?.style.position).toBe("fixed");
    expect(host.contains(popover)).toBe(false);
  });
});
