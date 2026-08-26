// @vitest-environment jsdom
import { act, createElement, Fragment } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";

import {
  createWorkbenchDisclosureStore,
  rendererWorkbenchDisclosureStore,
  useAutomaticWorkbenchDisclosure,
  useWorkbenchDisclosure,
  type WorkbenchDisclosureStore,
} from "./workbenchDisclosureStore";

describe("Workbench disclosure store", () => {
  it("notifies only the toggled stable identity", () => {
    const store = createWorkbenchDisclosureStore();
    const toolListener = vi.fn();
    const otherListener = vi.fn();
    store.subscribe("tool:read-1", toolListener);
    store.subscribe("tool:read-2", otherListener);

    store.set("tool:read-1", true);

    expect(store.getSnapshot("tool:read-1")).toBe(true);
    expect(store.getSnapshot("tool:read-2")).toBeUndefined();
    expect(toolListener).toHaveBeenCalledTimes(1);
    expect(otherListener).not.toHaveBeenCalled();
  });

  it("retains a choice while its leaf has no subscribers", () => {
    const store = createWorkbenchDisclosureStore();
    const listener = vi.fn();
    const unsubscribe = store.subscribe("execution:turn-1", listener);
    store.set("execution:turn-1", true);
    unsubscribe();

    expect(store.getSnapshot("execution:turn-1")).toBe(true);
    store.set("execution:turn-1", false);
    expect(listener).toHaveBeenCalledTimes(1);
    expect(store.getSnapshot("execution:turn-1")).toBe(false);
  });

  it("retains renderer-lifecycle choices across a complete consumer remount", async () => {
    const disclosureKey = "execution:renderer-lifecycle-remount-test";
    rendererWorkbenchDisclosureStore.set(disclosureKey, true);

    function Leaf() {
      const [open] = useWorkbenchDisclosure(
        rendererWorkbenchDisclosureStore,
        disclosureKey,
      );
      return createElement("span", null, String(open));
    }

    const firstContainer = document.createElement("div");
    document.body.append(firstContainer);
    const firstRoot = createRoot(firstContainer);
    await act(async () => firstRoot.render(createElement(Leaf)));
    expect(firstContainer.textContent).toBe("true");
    await act(async () => firstRoot.unmount());
    firstContainer.remove();

    const secondContainer = document.createElement("div");
    document.body.append(secondContainer);
    const secondRoot = createRoot(secondContainer);
    await act(async () => secondRoot.render(createElement(Leaf)));
    expect(secondContainer.textContent).toBe("true");
    await act(async () => secondRoot.unmount());
    secondContainer.remove();
    rendererWorkbenchDisclosureStore.set(disclosureKey, false);
  });

  it("rehydrates choices after the page module store is recreated", () => {
    const values = new Map<string, string>();
    const storage = {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => {
        values.set(key, value);
      },
    };
    const beforeNavigation = createWorkbenchDisclosureStore(storage);
    beforeNavigation.set("thread:one:turn:3:execution", true);
    beforeNavigation.set("thread:one:turn:3:tool:0", false);

    const afterNavigation = createWorkbenchDisclosureStore(storage);

    expect(afterNavigation.getSnapshot("thread:one:turn:3:execution")).toBe(true);
    expect(afterNavigation.getSnapshot("thread:one:turn:3:tool:0")).toBe(false);
    expect(afterNavigation.getSnapshot("thread:one:turn:4:execution")).toBeUndefined();
  });

  it("preserves a live batch's automatic open state across replay remount", async () => {
    const store = createWorkbenchDisclosureStore();
    const disclosureKey = "thread:one:turn:run-1:batch:tool-1";

    function Batch({ active, replayed }: { active: boolean; replayed: boolean }) {
      const [open] = useAutomaticWorkbenchDisclosure(store, disclosureKey, {
        active,
        replayed,
      });
      return createElement("span", null, String(open ?? active));
    }

    const firstContainer = document.createElement("div");
    const firstRoot = createRoot(firstContainer);
    await act(async () => {
      firstRoot.render(createElement(Batch, { active: true, replayed: false }));
    });
    expect(firstContainer.textContent).toBe("true");
    expect(store.getSnapshot(disclosureKey)).toBe(true);
    await act(async () => firstRoot.unmount());

    const replayContainer = document.createElement("div");
    const replayRoot = createRoot(replayContainer);
    await act(async () => {
      replayRoot.render(createElement(Batch, { active: false, replayed: true }));
    });
    expect(replayContainer.textContent).toBe("true");
    expect(store.getSnapshot(disclosureKey)).toBe(true);
    await act(async () => replayRoot.unmount());
  });

  it("keeps live automatic completion and explicit user choices authoritative", async () => {
    const store = createWorkbenchDisclosureStore();
    const disclosureKey = "thread:one:turn:run-2:batch:tool-2";

    function Batch({ active }: { active: boolean }) {
      const [open, setOpen] = useAutomaticWorkbenchDisclosure(store, disclosureKey, {
        active,
        replayed: false,
      });
      return createElement(
        "button",
        { type: "button", onClick: () => setOpen(false) },
        String(open ?? active),
      );
    }

    const container = document.createElement("div");
    const root = createRoot(container);
    await act(async () => {
      root.render(createElement(Batch, { active: true }));
    });
    expect(container.textContent).toBe("true");

    await act(async () => {
      root.render(createElement(Batch, { active: false }));
    });
    expect(container.textContent).toBe("false");

    await act(async () => {
      root.render(createElement(Batch, { active: true }));
    });
    await act(async () => {
      container.querySelector("button")?.click();
    });
    expect(store.getSnapshot(disclosureKey)).toBe(false);

    await act(async () => {
      root.render(createElement(Batch, { active: true }));
    });
    expect(container.textContent).toBe("false");
    await act(async () => root.unmount());
  });

  it("does not notify when the value is unchanged", () => {
    const store = createWorkbenchDisclosureStore();
    const listener = vi.fn();
    store.subscribe("batch:one", listener);
    store.set("batch:one", false);
    store.set("batch:one", false);

    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("rerenders only the leaf subscribed to the changed key", async () => {
    const store = createWorkbenchDisclosureStore();
    const renders = { first: 0, second: 0 };
    const container = document.createElement("div");
    document.body.append(container);
    const root = createRoot(container);

    function Leaf({
      disclosureKey,
      name,
      targetStore,
    }: {
      disclosureKey: string;
      name: keyof typeof renders;
      targetStore: WorkbenchDisclosureStore;
    }) {
      const [open] = useWorkbenchDisclosure(targetStore, disclosureKey);
      renders[name] += 1;
      return createElement("span", { "data-name": name }, String(open));
    }

    await act(async () => {
      root.render(
        createElement(
          Fragment,
          null,
          createElement(Leaf, {
            disclosureKey: "tool:first",
            name: "first",
            targetStore: store,
          }),
          createElement(Leaf, {
            disclosureKey: "tool:second",
            name: "second",
            targetStore: store,
          }),
        ),
      );
    });
    const initialRenders = { ...renders };

    await act(async () => store.set("tool:first", true));

    expect(renders.first).toBe(initialRenders.first + 1);
    expect(renders.second).toBe(initialRenders.second);
    expect(container.querySelector('[data-name="first"]')?.textContent).toBe("true");
    await act(async () => root.unmount());
    container.remove();
  });
});
