import type { ArtifactPreviewSelection } from "./file-preview/ArtifactPreviewHost";
import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";
import type { ProjectFileWorkspaceTab } from "./projectFileTabs";
import { artifactPreviewTabId } from "./artifactPreviewTabs";

export type InspectorFileTab = { kind: "file"; id: string; projectId?: string; mode: ProjectFileWorkspaceMode; tab?: ProjectFileWorkspaceTab };
export type InspectorArtifactTab = { kind: "artifact"; id: string; selection: ArtifactPreviewSelection };
export type InspectorTab = InspectorFileTab | InspectorArtifactTab | { kind: "subagent"; id: string; taskId: string };
export type InspectorTabsState = {
  threadId?: string;
  tabs: InspectorTab[];
  activeId?: string;
  // The mounted file viewer keeps its content while another surface is active.
  previewId?: string;
  nextOrdinal: number;
};
export type InspectorTabsCommand = (
  | { type: "open-subagent"; taskId: string }
  | { type: "open-artifact"; selection: ArtifactPreviewSelection }
  | { type: "open-file"; tab: ProjectFileWorkspaceTab }
  | { type: "add-file"; projectId?: string; mode: ProjectFileWorkspaceMode }
  | { type: "activate" | "close"; id: string }
  | { type: "set-mode"; mode: ProjectFileWorkspaceMode }
  | { type: "clear-file" }
  | { type: "project"; projectId?: string }
  | { type: "chooser" | "sync-thread" | "remove-subagents" | "remove-workspace" }
);
export type InspectorTabsAction = InspectorTabsCommand & { threadId?: string };
export const createInspectorTabs = (threadId?: string): InspectorTabsState => ({ threadId, tabs: [], nextOrdinal: 1 });
export const subagentTabId = (taskId: string) => `subagent:${taskId}`;
export function selectInspectorTab(state: InspectorTabsState, id?: string): InspectorTabsState {
  const tab = state.tabs.find(tab => tab.id === id);
  return { ...state, activeId: id, previewId: tab && tab.kind !== "subagent" ? id : state.previewId };
}
function retainTabs(state: InspectorTabsState, keep: (tab: InspectorTab) => boolean): InspectorTabsState {
  const tabs = state.tabs.filter(keep);
  const previousIndex = state.tabs.findIndex(tab => tab.id === state.activeId);
  const activeId = state.activeId === "chooser" || tabs.some(tab => tab.id === state.activeId)
    ? state.activeId : (state.tabs.slice(previousIndex + 1).find(keep) ?? state.tabs.slice(0, previousIndex).reverse().find(keep))?.id;
  const previewId = tabs.some(tab => tab.id === state.previewId) ? state.previewId : tabs.findLast(tab => tab.kind !== "subagent")?.id;
  return selectInspectorTab({ ...state, tabs, previewId }, activeId);
}
export function inspectorTabsForThread(state: InspectorTabsState, threadId?: string): InspectorTabsState {
  return state.threadId === threadId ? state : { ...retainTabs({ ...state, activeId: state.activeId === "chooser" ? undefined : state.activeId }, tab => tab.kind === "file"), threadId };
}
export function inspectorTabsReducer(previous: InspectorTabsState, action: InspectorTabsAction): InspectorTabsState {
  const state = inspectorTabsForThread(previous, action.threadId);
  const open = (tab: InspectorTab) => selectInspectorTab({ ...state,
    tabs: state.tabs.some(item => item.id === tab.id) ? state.tabs.map(item => item.id === tab.id ? tab : item) : [...state.tabs, tab],
  }, tab.id);
  switch (action.type) {
    case "sync-thread": return state;
    case "chooser": return { ...state, activeId: "chooser" };
    case "activate": return state.tabs.some(tab => tab.id === action.id) ? selectInspectorTab(state, action.id) : state;
    case "close": return retainTabs(state, tab => tab.id !== action.id);
    case "remove-subagents": return retainTabs(state, tab => tab.kind !== "subagent");
    case "remove-workspace": return retainTabs(state, tab => tab.kind === "subagent");
    case "project": return retainTabs(state, tab => tab.kind !== "file" || tab.projectId === action.projectId);
    case "open-subagent": return open({ kind: "subagent", id: subagentTabId(action.taskId), taskId: action.taskId });
    case "open-artifact": return open({ kind: "artifact", id: artifactPreviewTabId(action.selection), selection: action.selection });
    case "add-file": {
      const next = open({ kind: "file", id: `project-file-slot:${state.nextOrdinal}`, projectId: action.projectId, mode: action.mode });
      return { ...next, nextOrdinal: state.nextOrdinal + 1 };
    }
    case "open-file": {
      const current = state.tabs.find(tab => tab.kind === "file" && tab.tab?.id === action.tab.id)
        ?? state.tabs.find(tab => tab.id === state.activeId && tab.kind === "file" && tab.projectId === action.tab.projectId);
      const next = open({ kind: "file", id: current?.id ?? `project-file-slot:${state.nextOrdinal}`, projectId: action.tab.projectId, tab: action.tab, mode: action.tab.mode });
      return { ...next, nextOrdinal: state.nextOrdinal + (current ? 0 : 1) };
    }
    case "set-mode": return { ...state, tabs: state.tabs.map(tab => tab.id === state.activeId && tab.kind === "file" ? { ...tab, mode: action.mode } : tab) };
    case "clear-file": return { ...state, tabs: state.tabs.map(tab => tab.id === state.activeId && tab.kind === "file" ? { ...tab, tab: undefined, mode: "browser" } : tab) };
  }
}
