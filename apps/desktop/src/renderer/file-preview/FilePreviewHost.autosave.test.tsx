/** @vitest-environment jsdom */
import React, { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { EditorView } from "@codemirror/view";
import { afterEach, beforeEach, it, expect, vi } from "vitest";
import type { ProjectFilePreviewTextHandle } from "../../shared/runtime";
import CodePreview, { type CodePreviewProps } from "./CodePreview";
import FilePreviewHost, { type FilePreviewHostProps } from "./FilePreviewHost";

vi.mock("./previewLanguage", () => ({ loadPreviewLanguage: async () => null }));

let root: Root;
let host: HTMLDivElement;
beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

const preview: ProjectFilePreviewTextHandle = {
  previewId: "A", requestId: "req-A", kind: "code", transport: "text",
  displayName: "a.txt", relativePath: "a.txt", byteLength: 8,
  content: "original", contentDigest: "digest-original",
};
async function render(element: React.ReactNode) {
  await act(async () => { root.render(element); });
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 30)); });
}
function editor() {
  return EditorView.findFromDOM(host.querySelector(".cm-editor") as HTMLElement)!;
}

it("keeps saved text when a run starts and ends", async () => {
  const onSave = vi.fn(async () => ({ ok: true as const, byteLength: 14 }));
  const props: CodePreviewProps = {
    preview, editable: true, onSave,
    frameProps: { displayName: "a.txt", onReload: vi.fn(), onOpenExternal: vi.fn(), onReveal: vi.fn() },
  };
  await render(<CodePreview {...props} commentDisabled={false} />);
  await act(async () => editor().dispatch({ changes: { from: 0, to: 8, insert: "edited content" } }));
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 500)); });
  expect(onSave).toHaveBeenCalledWith("edited content");
  await render(<CodePreview {...props} commentDisabled={true} />);
  expect(editor().state.doc.toString()).toBe("edited content");
  await render(<CodePreview {...props} commentDisabled={false} />);
  expect(editor().state.doc.toString()).toBe("edited content");
});

it("flushes A to A when selection changes before autosave", async () => {
  const save = vi.fn<FilePreviewHostProps["runtimeClient"]["saveProjectMarkdownFile"]>(async () => ({ ok: true, digest: "next", byteLength: 9 }));
  const client: FilePreviewHostProps["runtimeClient"] = {
    closeProjectFilePreview: vi.fn(async () => undefined),
    startProjectFilePreview: vi.fn<FilePreviewHostProps["runtimeClient"]["startProjectFilePreview"]>(async (request) => ({
      ok: true, preview: { ...preview, previewId: request.relativePath, displayName: request.relativePath },
    })),
    saveProjectMarkdownFile: save,
    openProjectFileExternally: vi.fn(async () => ({ ok: true as const })),
    revealProjectFile: vi.fn(async () => ({ ok: true as const })),
  };
  const props: FilePreviewHostProps = {
    runtimeClient: client, projectId: "p", folderPath: "/tmp/autosave-fixture", mode: "files", onModeChange: vi.fn(),
  };
  await render(<FilePreviewHost {...props} selection={{ name: "a.txt", relativePath: "a.txt" }} />);
  await act(async () => editor().dispatch({ changes: { from: 0, to: 8, insert: "A changed" } }));
  await render(<FilePreviewHost {...props} selection={{ name: "b.txt", relativePath: "b.txt" }} />);
  expect(save).toHaveBeenCalledTimes(1);
  expect(save).toHaveBeenCalledWith({
    projectId: "p", folderPath: "/tmp/autosave-fixture", relativePath: "a.txt",
    content: "A changed", expectedDigest: "digest-original",
  });
  expect(editor().state.doc.toString()).toBe("original");
});

it("navigates once to a source range without reopening the preview or creating a comment", async () => {
  const content = Array.from({ length: 100 }, (_, i) => `line ${i + 1}`).join("\n");
  const start = vi.fn<FilePreviewHostProps["runtimeClient"]["startProjectFilePreview"]>(async () => ({ ok: true, preview: { ...preview, content } }));
  const client: FilePreviewHostProps["runtimeClient"] = {
    closeProjectFilePreview: vi.fn(async () => undefined), startProjectFilePreview: start,
    saveProjectMarkdownFile: vi.fn(async () => ({ ok: true as const, digest: "next", byteLength: content.length })),
    openProjectFileExternally: vi.fn(async () => ({ ok: true as const })), revealProjectFile: vi.fn(async () => ({ ok: true as const })),
  };
  const props: FilePreviewHostProps = { runtimeClient: client, projectId: "p", folderPath: "/tmp/fixture", mode: "files", onModeChange: vi.fn() };
  const request = { id: "locate-1", startLine: 40, endLine: 47 };
  const view = (lineRequest = request) => <FilePreviewHost {...props} selection={{ name: "a.txt", relativePath: "a.txt" }} lineRequest={lineRequest} />;
  await render(view());
  const instance = editor();
  expect(instance.state.doc.lineAt(instance.state.selection.main.from).number).toBe(40);
  expect(instance.state.doc.lineAt(instance.state.selection.main.to).number).toBe(47);
  expect(host.querySelector("textarea")).toBeNull();
  await act(async () => instance.dispatch({ selection: { anchor: 0 }, changes: { from: 0, to: 4, insert: "edit" } }));
  await render(view({ ...request }));
  expect(editor()).toBe(instance);
  expect(start).toHaveBeenCalledTimes(1);
  expect(instance.state.selection.main.from).toBe(0);
  expect(instance.state.doc.toString()).toMatch(/^edit/);
  await render(view({ id: "locate-2", startLine: 200, endLine: 250 }));
  expect(instance.state.doc.lineAt(instance.state.selection.main.from).number).toBe(100);
  expect(start).toHaveBeenCalledTimes(1);
  expect(host.querySelector("textarea")).toBeNull();
});
