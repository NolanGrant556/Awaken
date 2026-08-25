import { describe, expect, it } from "vitest";
import {
  activeProjectFileTab,
  activeProjectFileTabSlot,
  createProjectFileTabsState,
  projectFileTabId,
  projectFileTabsReducer,
  type ProjectFileWorkspaceTab,
} from "./projectFileTabs";

function tab(name: string): ProjectFileWorkspaceTab {
  return {
    id: projectFileTabId("project-1", "/workspace", name),
    projectId: "project-1",
    folderPath: "/workspace",
    name,
    relativePath: name,
    mode: name.endsWith(".html") ? "browser" : "files",
  };
}

describe("projectFileTabsReducer", () => {
  it("reuses the active slot when different files are opened", () => {
    const readme = tab("README.md");
    const image = tab("page-07.png");
    let state = projectFileTabsReducer(createProjectFileTabsState(), {
      type: "open",
      tab: readme,
    });
    state = projectFileTabsReducer(state, { type: "open", tab: image });

    expect(state.tabs).toHaveLength(1);
    expect(activeProjectFileTab(state)?.name).toBe("page-07.png");
    expect(activeProjectFileTabSlot(state)?.id).toBe("project-file-slot:1");
  });

  it("adds an empty slot explicitly and fills only the active slot", () => {
    const first = tab("first.md");
    const second = tab("second.pdf");
    const third = tab("third.html");
    let state = projectFileTabsReducer(createProjectFileTabsState(), {
      type: "open",
      tab: first,
    });
    state = projectFileTabsReducer(state, { type: "add", projectId: "project-1" });
    expect(activeProjectFileTab(state)).toBeUndefined();
    state = projectFileTabsReducer(state, { type: "open", tab: second });
    state = projectFileTabsReducer(state, { type: "add", projectId: "project-1" });
    state = projectFileTabsReducer(state, { type: "open", tab: third });

    expect(state.tabs.map((slot) => slot.tab?.name)).toEqual([
      "first.md",
      "second.pdf",
      "third.html",
    ]);

    state = projectFileTabsReducer(state, {
      type: "close",
      tabId: "project-file-slot:2",
    });
    expect(activeProjectFileTab(state)?.id).toBe(third.id);

    state = projectFileTabsReducer(state, {
      type: "close",
      tabId: "project-file-slot:3",
    });
    expect(activeProjectFileTab(state)?.id).toBe(first.id);
  });

  it("keeps the active tab when a background tab closes and clears on reset", () => {
    const first = tab("first.md");
    const second = tab("second.pdf");
    let state = projectFileTabsReducer(createProjectFileTabsState(), {
      type: "open",
      tab: first,
    });
    state = projectFileTabsReducer(state, { type: "add", projectId: "project-1" });
    state = projectFileTabsReducer(state, { type: "open", tab: second });
    state = projectFileTabsReducer(state, {
      type: "close",
      tabId: "project-file-slot:1",
    });

    expect(activeProjectFileTab(state)?.id).toBe(second.id);
    expect(projectFileTabsReducer(state, { type: "reset" })).toEqual({
      tabs: [],
      nextTabOrdinal: 1,
    });
  });
});
