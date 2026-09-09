/** @vitest-environment jsdom */
import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { useThreadNavigation } from "./useThreadNavigation";
it.each(["archive", "delete"])("late %s response for A must not reset newly selected B", async (action) => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  const root = createRoot(document.createElement("div"));
  let controller: any;
  let resolve!: (v: any) => void;
  const pending = new Promise(r => { resolve = r; });
  const reset = vi.fn();
  const closeFile = vi.fn();
  const opts: any = {
    dispatch: vi.fn(),
    runtimeClient: { setThreadArchived: () => pending, deleteThreads: () => pending },
    desktopReady: true,
    directory: { refreshThreads: async () => {}, loadThreadsIntoState: async () => [] },
    setConversationSectionExpanded: vi.fn(),
    clearThreadComposerDraft: vi.fn(),
    setNotice: vi.fn(),
    resetActiveThreadContext: reset,
    onActiveThreadArchived: closeFile,
    threadsForProject: () => [],
  };
  const threads = [{ threadId: "A", label: "A", status: "idle" }, { threadId: "B", label: "B", status: "idle" }];
  function Harness({ selected }: { selected: string }) {
    controller = useThreadNavigation({ ...opts, runtimeState: { threads, activeThreadId: selected } });
    return null;
  }
  try {
    await act(async () => { root.render(createElement(Harness, { selected: "A" })); });
    let work!: Promise<void>;
    if (action === "archive") {
      await act(async () => { work = controller.handleArchiveThread("A"); });
    }
    else {
      await act(async () => { controller.requestThreadDelete(["A"], "single"); });
      await act(async () => { work = controller.handleConfirmThreadDelete(); });
    }
    await act(async () => { root.render(createElement(Harness, { selected: "B" })); });
    await act(async () => { resolve({ ok: true, deletedThreadIds: ["A"], failures: [] }); await work; });
    expect(reset, "A response reset B's active thread context").not.toHaveBeenCalled();
    expect(closeFile, "A response closed B's file workspace").not.toHaveBeenCalled();
  }
  finally {
    await act(async () => root.unmount());
    vi.unstubAllGlobals();
  }
});
