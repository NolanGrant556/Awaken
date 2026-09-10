/** @vitest-environment jsdom */
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { useRuntimeInteractions } from "./useRuntimeInteractions";

describe("background interaction ownership", () => {
  it.each(["new-run", undefined])("routes retained controls to the old run when current run is %s", async (runId) => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    const host = document.createElement("div");
    const root = createRoot(host);
    const client = { resolveExternalInteraction: vi.fn(), resolveACPPermission: vi.fn(), resolveComputerReview: vi.fn(), computerSessionControl: vi.fn() };
    let handlers!: ReturnType<typeof useRuntimeInteractions>;
    function Harness() {
      handlers = useRuntimeInteractions({ runId, runtimeClient: client, setNotice: vi.fn(),
        activeComputerReview: { id: "review", reviewId: "review", runId: "old-run", taskId: "child" },
        activeComputerHandoff: { id: "handoff", runId: "old-run", taskId: "child", sessionId: "old-session", active: true },
      });
      return null;
    }
    try {
      await act(async () => root.render(<Harness />));
      await act(async () => handlers.resolveActiveComputerReview("denied"));
      await act(async () => handlers.returnComputerControl());
      expect(client.resolveComputerReview).toHaveBeenCalledWith("old-run", "review", "denied");
      expect(client.computerSessionControl).toHaveBeenCalledWith("old-run", "return", "old-session");
    } finally { await act(async () => root.unmount()); }
  });
  it("does not guess the new run for an incomplete child identity", async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    const root = createRoot(document.createElement("div"));
    const client = { resolveExternalInteraction: vi.fn(), resolveACPPermission: vi.fn(), resolveComputerReview: vi.fn(), computerSessionControl: vi.fn() };
    let handlers!: ReturnType<typeof useRuntimeInteractions>;
    function Harness() {
      handlers = useRuntimeInteractions({ runId: "new-run", runtimeClient: client, setNotice: vi.fn(),
        activeComputerReview: { id: "review", reviewId: "review", taskId: "child" },
        activeComputerHandoff: { id: "handoff", taskId: "child", active: true },
      });
      return null;
    }
    try {
      await act(async () => root.render(<Harness />));
      await act(async () => handlers.resolveActiveComputerReview("denied"));
      await act(async () => handlers.returnComputerControl());
      expect(client.resolveComputerReview).not.toHaveBeenCalled();
      expect(client.computerSessionControl).not.toHaveBeenCalled();
    } finally { await act(async () => root.unmount()); }
  });
});
