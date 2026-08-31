import { EditorState } from "@codemirror/state";
import { describe, expect, it } from "vitest";

import { codePreviewLineSelection } from "./CodePreview";

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
