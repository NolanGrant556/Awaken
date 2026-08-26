import type { ArtifactPreviewSelection } from "./file-preview/ArtifactPreviewHost";

export type ArtifactPreviewTab = {
  id: string;
  selection: ArtifactPreviewSelection;
};

export type ArtifactPreviewTabsState = {
  tabs: ArtifactPreviewTab[];
  activeTabId?: string;
};

export type ArtifactPreviewTabsAction =
  | { type: "open"; selection: ArtifactPreviewSelection }
  | { type: "activate"; tabId: string }
  | { type: "deactivate" }
  | { type: "close"; tabId: string }
  | { type: "reset" };

export function artifactPreviewTabId(
  selection: Pick<ArtifactPreviewSelection, "threadId" | "runId" | "artifactId">,
): string {
  return JSON.stringify([
    selection.threadId,
    selection.runId,
    selection.artifactId,
  ]);
}

export function createArtifactPreviewTabsState(): ArtifactPreviewTabsState {
  return { tabs: [] };
}

export function artifactPreviewTabsReducer(
  state: ArtifactPreviewTabsState,
  action: ArtifactPreviewTabsAction,
): ArtifactPreviewTabsState {
  if (action.type === "reset") {
    return createArtifactPreviewTabsState();
  }

  if (action.type === "open") {
    const id = artifactPreviewTabId(action.selection);
    const existingIndex = state.tabs.findIndex((tab) => tab.id === id);
    const tabs =
      existingIndex >= 0
        ? state.tabs.map((tab, index) =>
            index === existingIndex
              ? { ...tab, selection: action.selection }
              : tab,
          )
        : [...state.tabs, { id, selection: action.selection }];
    return { tabs, activeTabId: id };
  }

  if (action.type === "activate") {
    if (!state.tabs.some((tab) => tab.id === action.tabId)) return state;
    return { ...state, activeTabId: action.tabId };
  }

  if (action.type === "deactivate") {
    if (!state.activeTabId) return state;
    return { ...state, activeTabId: undefined };
  }

  const closingIndex = state.tabs.findIndex((tab) => tab.id === action.tabId);
  if (closingIndex === -1) return state;
  const tabs = state.tabs.filter((tab) => tab.id !== action.tabId);
  if (state.activeTabId !== action.tabId) {
    return { ...state, tabs };
  }
  return {
    tabs,
    activeTabId: tabs[closingIndex]?.id ?? tabs[closingIndex - 1]?.id,
  };
}

export function activeArtifactPreviewTab(
  state: ArtifactPreviewTabsState,
): ArtifactPreviewTab | undefined {
  return state.tabs.find((tab) => tab.id === state.activeTabId);
}
