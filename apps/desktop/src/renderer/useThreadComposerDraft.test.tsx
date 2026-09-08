/** @vitest-environment jsdom */
import { act, createElement, StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useThreadComposerDraft, type ThreadComposerDraft } from "./useThreadComposerDraft";

let root: Root;
let controller: ReturnType<typeof useThreadComposerDraft>;
function Harness({ owner }: { owner: string }) {
  controller = useThreadComposerDraft(owner);
  return null;
}
async function select(owner: string) {
  await act(async () => {
    root.render(createElement(StrictMode, null, createElement(Harness, { owner })));
  });
}
beforeEach(async () => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  root = createRoot(document.createElement("div"));
  await select("A");
});
afterEach(async () => { await act(async () => root.unmount()); vi.unstubAllGlobals(); });

it("recovers the whole submitted draft in A while preserving B", async () => {
  await act(async () => {
    controller.setValue("original");
    controller.setTokens([{ id: "skill:one", type: "skill", label: "one", leadingText: "使用 " }]);
    controller.setTokenPlacements({ "skill:one": "before" });
    controller.setTokenLeadingTexts({ "skill:one": "使用 " });
  });
  const original = controller.draft;
  let restore!: (draft: ThreadComposerDraft) => void;
  await act(async () => { restore = controller.clearForSubmission(); });
  // The input's automatic caret update must not invalidate recovery.
  await act(async () => { controller.setSelection({ anchor: 0, focus: 0 }); });
  await select("B");
  await act(async () => { controller.setValue("B's own draft"); restore(original); });
  expect(controller.draft.value).toBe("B's own draft");
  await select("A");
  expect(controller.draft).toEqual(original);
});

it.each(["edit", "clear", "resubmit"] as const)("does not overwrite a later %s in A", async (action) => {
  await act(async () => { controller.setValue("old"); });
  const original = controller.draft;
  let restore!: (draft: ThreadComposerDraft) => void;
  await act(async () => { restore = controller.clearForSubmission(); });
  await select("B");
  await select("A");
  await act(async () => {
    if (action === "edit") controller.setValue("new");
    if (action === "clear") controller.clearDraft();
    if (action === "resubmit") controller.clearForSubmission();
    restore(original);
  });
  expect(controller.draft.value).toBe(action === "edit" ? "new" : "");
});

it("invalidates old recovery even when newer text is typed and erased", async () => {
  const original = { ...controller.draft, value: "old" };
  let restore!: (draft: ThreadComposerDraft) => void;
  await act(async () => { restore = controller.clearForSubmission(); });
  await act(async () => { controller.setValue("new"); controller.setValue(""); restore(original); });
  expect(controller.draft.value).toBe("");
});
