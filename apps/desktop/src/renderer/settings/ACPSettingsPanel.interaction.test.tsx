// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RuntimeClient } from "../runtime/client";
import { ACPSettingsPanel } from "./ACPSettingsPanel";

describe("ACPSettingsPanel editor dialog", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.append(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    document.body.replaceChildren();
    vi.restoreAllMocks();
  });

  it("uses the shared centered dialog for both create and edit flows", async () => {
    const runtimeClient = {
      getACPRunners: vi.fn(async () => ({ runners: [] })),
    } as unknown as RuntimeClient;

    await act(async () => {
      root.render(<ACPSettingsPanel runtimeClient={runtimeClient} />);
      await Promise.resolve();
    });

    const addButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("添加 runner"),
    );
    expect(addButton).toBeDefined();

    addButton?.focus();
    await act(async () => addButton?.click());

    const createDialog = document.querySelector<HTMLElement>(
      ".aw-dialog.acp-runner-dialog",
    );
    expect(createDialog).not.toBeNull();
    expect(createDialog?.textContent).toContain("添加 ACP runner");
    expect(document.querySelector(".acp-runner-drawer")).toBeNull();
    expect(document.activeElement).toBe(
      createDialog?.querySelector('input[type="text"]'),
    );

    await act(async () => {
      document.dispatchEvent(
        new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
      );
    });
    expect(document.querySelector(".acp-runner-dialog")).toBeNull();
    expect(document.activeElement).toBe(addButton);

    const firstRunner =
      container.querySelector<HTMLElement>(".acp-runner-card");
    expect(firstRunner).not.toBeNull();
    firstRunner?.focus();
    await act(async () => firstRunner?.click());

    const editDialog = document.querySelector<HTMLElement>(
      ".aw-dialog.acp-runner-dialog",
    );
    expect(editDialog?.textContent).toContain("编辑 opencode");
  });
});
