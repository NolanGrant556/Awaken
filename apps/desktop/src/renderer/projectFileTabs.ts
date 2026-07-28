import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";

export type ProjectFileWorkspaceTab = {
  id: string;
  projectId: string;
  folderPath: string;
  name: string;
  relativePath: string;
  mode: ProjectFileWorkspaceMode;
};

export type ProjectFileTabsState = {
  tabs: ProjectFileWorkspaceTab[];
  activeTabId?: string;
};

export type ProjectFileTabsAction =
  | { type: "open"; tab: ProjectFileWorkspaceTab }
  | { type: "activate"; tabId: string }
  | { type: "close"; tabId: string }
  | { type: "reset" };

export function createProjectFileTabsState(): ProjectFileTabsState {
  return { tabs: [] };
}

export function projectFileTabId(
  projectId: string,
  folderPath: string,
  relativePath: string,
): string {
  return JSON.stringify([projectId, folderPath, relativePath]);
}

export function projectFileTabsReducer(
  state: ProjectFileTabsState,
  action: ProjectFileTabsAction,
): ProjectFileTabsState {
  if (action.type === "reset") {
    return createProjectFileTabsState();
  }

  if (action.type === "open") {
    const existingIndex = state.tabs.findIndex((tab) => tab.id === action.tab.id);
    const tabs =
      existingIndex === -1
        ? [...state.tabs, action.tab]
        : state.tabs.map((tab, index) =>
            index === existingIndex ? action.tab : tab,
          );
    return { tabs, activeTabId: action.tab.id };
  }

  if (action.type === "activate") {
    if (!state.tabs.some((tab) => tab.id === action.tabId)) return state;
    return { ...state, activeTabId: action.tabId };
  }

  const closingIndex = state.tabs.findIndex((tab) => tab.id === action.tabId);
  if (closingIndex === -1) return state;
  const tabs = state.tabs.filter((tab) => tab.id !== action.tabId);
  if (state.activeTabId !== action.tabId) {
    return { tabs, activeTabId: state.activeTabId };
  }
  return {
    tabs,
    activeTabId: tabs[closingIndex]?.id ?? tabs[closingIndex - 1]?.id,
  };
}

export function activeProjectFileTab(
  state: ProjectFileTabsState,
): ProjectFileWorkspaceTab | undefined {
  return state.tabs.find((tab) => tab.id === state.activeTabId);
}
