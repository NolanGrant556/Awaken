// @vitest-environment jsdom

import { beforeEach, describe, expect, it } from "vitest";
import { selectAllInFocusedEditor } from "./desktop-selection";

describe("selectAllInFocusedEditor", () => {
  beforeEach(() => {
    document.body.replaceChildren();
    document.getSelection()?.removeAllRanges();
  });

  it("selects only the focused textarea draft", () => {
    const textarea = document.createElement("textarea");
    textarea.value = "Awaken draft";
    document.body.append(textarea);
    textarea.focus();

    expect(selectAllInFocusedEditor(document)).toBe(true);
    expect(textarea.selectionStart).toBe(0);
    expect(textarea.selectionEnd).toBe(textarea.value.length);
  });

  it("selects only the focused text input", () => {
    const input = document.createElement("input");
    input.value = "Model profile";
    document.body.append(input);
    input.focus();

    expect(selectAllInFocusedEditor(document)).toBe(true);
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe(input.value.length);
  });

  it("selects the single plaintext editing host used by token input", () => {
    const editor = document.createElement("div");
    editor.setAttribute("contenteditable", "plaintext-only");
    editor.tabIndex = 0;
    editor.append("before ");
    const token = document.createElement("span");
    token.setAttribute("contenteditable", "false");
    token.textContent = "Skill";
    editor.append(token, " after");
    document.body.append(editor);
    editor.focus();

    expect(selectAllInFocusedEditor(document)).toBe(true);
    expect(document.getSelection()?.toString()).toBe("before Skill after");
  });

  it("clears document selection when focus is outside an editor", () => {
    const chrome = document.createElement("button");
    chrome.textContent = "New Agent";
    const answer = document.createElement("p");
    answer.textContent = "Selectable answer";
    document.body.append(chrome, answer);
    const range = document.createRange();
    range.selectNodeContents(document.body);
    document.getSelection()?.addRange(range);
    chrome.focus();

    expect(selectAllInFocusedEditor(document)).toBe(false);
    expect(document.getSelection()?.isCollapsed).toBe(true);
  });
});
