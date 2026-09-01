import { EditorState } from "@codemirror/state";
import { describe, expect, it } from "vitest";

import {
  codePreviewCommentHighlightSelection,
  codePreviewLineSelection,
} from "./CodePreview";

describe("codePreviewLineSelection", () => {
  it("projects a cursor to its complete source line", () => {
    const state = EditorState.create({
      doc: "alpha\nbeta\ngamma",
      selection: { anchor: 8 },
    });

    expect(codePreviewLineSelection(state)).toEqual({
      startLine: 2,
      endLine: 2,
      selectedText: "beta",
    });
  });

  it("does not include the next line when selection ends at its start", () => {
    const state = EditorState.create({
      doc: "alpha\nbeta\ngamma",
      selection: { anchor: 0, head: 11 },
    });

    expect(codePreviewLineSelection(state)).toEqual({
      startLine: 1,
      endLine: 2,
      selectedText: "alpha\nbeta",
    });
  });
});

describe("codePreviewCommentHighlightSelection", () => {
  it("keeps the original multi-line range while the comment editor has focus", () => {
    const editing = {
      startLine: 9,
      endLine: 14,
      selectedText: "line 9\nline 10\nline 11\nline 12\nline 13\nline 14",
    };
    const collapsedSelection = {
      startLine: 14,
      endLine: 14,
      selectedText: "line 14",
    };

    expect(codePreviewCommentHighlightSelection(collapsedSelection, editing)).toBe(editing);
  });

  it("uses the live CodeMirror selection when no comment is being edited", () => {
    const selection = {
      startLine: 3,
      endLine: 5,
      selectedText: "line 3\nline 4\nline 5",
    };

    expect(codePreviewCommentHighlightSelection(selection, undefined)).toBe(selection);
  });
});
