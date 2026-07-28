import { describe, expect, it } from "vitest";
import {
  activeProjectFileTab,
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
  it("adds new files and activates an existing file without duplicating it", () => {
    const readme = tab("README.md");
    const image = tab("page-07.png");
    let state = projectFileTabsReducer(createProjectFileTabsState(), {
      type: "open",
      tab: readme,
    });
    state = projectFileTabsReducer(state, { type: "open", tab: image });
    state = projectFileTabsReducer(state, { type: "open", tab: readme });

    expect(state.tabs.map((item) => item.name)).toEqual([
      "README.md",
      "page-07.png",
    ]);
    expect(activeProjectFileTab(state)?.name).toBe("README.md");
  });

  it("closes one tab and selects the nearest remaining tab", () => {
    const first = tab("first.md");
    const second = tab("second.pdf");
    const third = tab("third.html");
    let state = [first, second, third].reduce(
      (current, item) => projectFileTabsReducer(current, { type: "open", tab: item }),
      createProjectFileTabsState(),
    );

    state = projectFileTabsReducer(state, { type: "close", tabId: second.id });
    expect(activeProjectFileTab(state)?.id).toBe(third.id);
    expect(state.tabs.map((item) => item.id)).toEqual([first.id, third.id]);

    state = projectFileTabsReducer(state, { type: "close", tabId: third.id });
    expect(activeProjectFileTab(state)?.id).toBe(first.id);
  });

  it("keeps the active tab when a background tab closes and clears on reset", () => {
    const first = tab("first.md");
    const second = tab("second.pdf");
    let state = projectFileTabsReducer(createProjectFileTabsState(), {
      type: "open",
      tab: first,
    });
    state = projectFileTabsReducer(state, { type: "open", tab: second });
    state = projectFileTabsReducer(state, { type: "close", tabId: first.id });

    expect(activeProjectFileTab(state)?.id).toBe(second.id);
    expect(projectFileTabsReducer(state, { type: "reset" })).toEqual({ tabs: [] });
  });
});
