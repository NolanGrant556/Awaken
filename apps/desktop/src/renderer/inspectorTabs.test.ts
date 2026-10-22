import { describe, expect, it } from "vitest";
import { createInspectorTabs, inspectorTabsReducer, type InspectorTabsCommand, type InspectorTabsState } from "./inspectorTabs";
import type { ProjectFileWorkspaceTab } from "./projectFileTabs";
const file = (name: string): ProjectFileWorkspaceTab => ({ id: name, projectId: "p", folderPath: "/fixture", relativePath: name, name, mode: "files" });
const artifact = (id: string) => ({ threadId: "t", runId: "r", artifactId: id, title: id });
const apply = (state: InspectorTabsState, command: InspectorTabsCommand, threadId = "t") => inspectorTabsReducer(state, { ...command, threadId });
describe("inspector tabs", () => {
  it("coalesces repeated host browser requests while retaining explicit user-created tabs", () => {
    let state = createInspectorTabs("t");
    for (let index = 0; index < 8; index++) {
      state = apply(state, { type: "show-browser", projectId: "p" });
    }
    expect(state.tabs).toHaveLength(1);
    const first = state.activeId!;
    state = apply(state, { type: "add-file", mode: "browser", projectId: "p" });
    const second = state.activeId!;
    state = apply(state, { type: "show-browser", viewId: first });
    expect(state.activeId).toBe(first);
    expect(state.tabs).toHaveLength(2);
    state = apply(state, { type: "close", id: first });
    state = apply(state, { type: "show-browser", viewId: first });
    expect(state.activeId).toBe(second);
    expect(state.tabs).toHaveLength(1);
  });
  it("reuses the same file when navigating from a child and passes a fresh line request", () => {
    let state = apply(createInspectorTabs(), { type: "open-file", tab: file("a") });
    const id = state.activeId;
    state = apply(state, { type: "open-subagent", taskId: "child" });
    state = apply(state, { type: "open-file", tab: { ...file("a"), lineRequest: { id: "location", startLine: 40, endLine: 47 } } });
    expect(state.tabs).toHaveLength(2);
    expect(state.activeId).toBe(id);
    expect(state.tabs[0]).toMatchObject({ tab: { lineRequest: { id: "location", startLine: 40 } } });
  });
  it("replaces only the active file slot and appends an explicitly requested slot", () => {
    let state = apply(createInspectorTabs(), { type: "open-file", tab: file("a") });
    const id = state.activeId;
    state = apply(state, { type: "open-file", tab: file("b") });
    expect(state.tabs).toHaveLength(1);
    expect(state.activeId).toBe(id);
    state = apply(state, { type: "add-file", projectId: "p", mode: "files" });
    state = apply(state, { type: "open-file", tab: file("c") });
    expect(state.tabs.map(tab => tab.kind === "file" && tab.tab?.name)).toEqual(["b", "c"]);
  });
  it("preserves blank browser identity and mode across chooser and child switching", () => {
    let state = apply(createInspectorTabs(), { type: "add-file", projectId: "p", mode: "browser" });
    const browserId = state.activeId!;
    state = apply(state, { type: "open-subagent", taskId: "child" });
    state = apply(state, { type: "chooser" });
    expect(state.tabs).toHaveLength(2);
    expect(state.previewId).toBe(browserId);
    state = apply(state, { type: "activate", id: browserId });
    expect(state.tabs.find(tab => tab.id === state.activeId)).toMatchObject({ kind: "file", mode: "browser" });
  });
  it("closes adjacent tabs across kinds in actual display order", () => {
    let state = apply(createInspectorTabs(), { type: "open-file", tab: file("a") });
    const first = state.activeId!;
    state = apply(state, { type: "open-subagent", taskId: "child" });
    const child = state.activeId!;
    state = apply(state, { type: "open-artifact", selection: artifact("report") });
    const last = state.activeId!;
    state = apply(state, { type: "activate", id: child });
    state = apply(state, { type: "close", id: child });
    expect(state.activeId).toBe(last);
    state = apply(state, { type: "close", id: last });
    expect(state.activeId).toBe(first);
    state = apply(state, { type: "close", id: first });
    expect(state.tabs).toEqual([]);
    expect(state.activeId).toBeUndefined();
    expect(state.previewId).toBeUndefined();
  });
  it("reuses exact child and artifact identities without merging same titles", () => {
    let state = createInspectorTabs("t");
    for (const taskId of ["a", "b", "a"]) state = apply(state, { type: "open-subagent", taskId });
    for (const artifactId of ["one", "two", "one"]) state = apply(state, { type: "open-artifact", selection: { ...artifact(artifactId), title: "same" } });
    expect(state.tabs).toHaveLength(4);
    expect(state.tabs.map(tab => tab.kind)).toEqual(["subagent", "subagent", "artifact", "artifact"]);
  });
  it("closing a background tab does not change current selection", () => {
    let state = apply(createInspectorTabs(), { type: "open-subagent", taskId: "one" });
    const old = state.activeId!;
    state = apply(state, { type: "add-file", projectId: "p", mode: "browser" });
    const active = state.activeId;
    state = apply(state, { type: "close", id: old });
    expect(state.activeId).toBe(active);
  });
  it("clears thread-owned tabs on thread change and project files on project change", () => {
    let state = apply(createInspectorTabs(), { type: "open-file", tab: file("a") });
    state = apply(state, { type: "open-subagent", taskId: "child" });
    state = apply(state, { type: "open-artifact", selection: artifact("report") });
    state = apply(state, { type: "sync-thread" }, "other");
    expect(state.tabs.map(tab => tab.kind)).toEqual(["file"]);
    expect(state.activeId).toBe(state.tabs[0].id);
    state = apply(state, { type: "project", projectId: "other" }, "other");
    expect(state.tabs).toEqual([]);
  });
  it("clears the HTML file from a browser slot without creating a new tab", () => {
    let state = apply(createInspectorTabs(), { type: "add-file", projectId: "p", mode: "browser" });
    const browserId = state.activeId;
    state = apply(state, { type: "open-file", tab: { ...file("index.html"), mode: "browser" } });
    expect(state.tabs.find(tab => tab.id === browserId)).toMatchObject({
      kind: "file",
      tab: { name: "index.html" },
      mode: "browser",
    });
    state = apply(state, { type: "clear-file" });
    expect(state.activeId).toBe(browserId);
    expect(state.tabs.find(tab => tab.id === browserId)).toMatchObject({
      kind: "file",
      tab: undefined,
      mode: "browser",
    });
  });
  it("keeps artifact tabs when selecting project files or adding a browser", () => {
    let state = apply(createInspectorTabs(), { type: "open-artifact", selection: artifact("report") });
    const report = state.tabs[0];
    state = apply(state, { type: "open-file", tab: file("a") });
    state = apply(state, { type: "add-file", projectId: "p", mode: "browser" });
    expect(state.tabs[0]).toEqual(report);
    state = apply(state, { type: "remove-workspace" });
    expect(state.tabs).toEqual([]);
  });
});
