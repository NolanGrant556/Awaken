import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";

export type ProjectFileWorkspaceTab = {
  id: string;
  projectId: string;
  folderPath: string;
  name: string;
  relativePath: string;
  mode: ProjectFileWorkspaceMode;
};

export type ProjectFileTabSlot = {
  id: string;
  projectId: string;
  tab?: ProjectFileWorkspaceTab;
};

export type ProjectFileTabsState = {
  tabs: ProjectFileTabSlot[];
  activeTabId?: string;
  nextTabOrdinal: number;
};

export type ProjectFileTabsAction =
  | { type: "open"; tab: ProjectFileWorkspaceTab }
  | { type: "add"; projectId: string }
  | { type: "activate"; tabId: string }
  | { type: "close"; tabId: string }
  | { type: "reset" };

export function createProjectFileTabsState(): ProjectFileTabsState {
  return { tabs: [], nextTabOrdinal: 1 };
}

export function projectFileTabId(
  projectId: string,
  folderPath: string,
  relativePath: string,
): string {
  return JSON.stringify([projectId, folderPath, relativePath]);
}

function nextProjectFileTabSlot(
  state: ProjectFileTabsState,
  projectId: string,
  tab?: ProjectFileWorkspaceTab,
): ProjectFileTabSlot {
  return {
    id: `project-file-slot:${state.nextTabOrdinal}`,
    projectId,
    tab,
  };
}

export function projectFileTabsReducer(
  state: ProjectFileTabsState,
  action: ProjectFileTabsAction,
): ProjectFileTabsState {
  if (action.type === "reset") {
    return createProjectFileTabsState();
  }

  if (action.type === "open") {
    const activeIndex = state.tabs.findIndex(
      (slot) =>
        slot.id === state.activeTabId && slot.projectId === action.tab.projectId,
    );
    if (activeIndex >= 0) {
      const tabs = state.tabs.map((slot, index) =>
        index === activeIndex ? { ...slot, tab: action.tab } : slot,
      );
      return { ...state, tabs };
    }
    const slot = nextProjectFileTabSlot(
      state,
      action.tab.projectId,
      action.tab,
    );
    return {
      tabs: [...state.tabs, slot],
      activeTabId: slot.id,
      nextTabOrdinal: state.nextTabOrdinal + 1,
    };
  }

  if (action.type === "add") {
    const slot = nextProjectFileTabSlot(state, action.projectId);
    return {
      tabs: [...state.tabs, slot],
      activeTabId: slot.id,
      nextTabOrdinal: state.nextTabOrdinal + 1,
    };
  }

  if (action.type === "activate") {
    if (!state.tabs.some((tab) => tab.id === action.tabId)) return state;
    return { ...state, activeTabId: action.tabId };
  }

  const closingIndex = state.tabs.findIndex((tab) => tab.id === action.tabId);
  if (closingIndex === -1) return state;
  const tabs = state.tabs.filter((tab) => tab.id !== action.tabId);
  if (state.activeTabId !== action.tabId) {
    return { ...state, tabs };
  }
  return {
    ...state,
    tabs,
    activeTabId: tabs[closingIndex]?.id ?? tabs[closingIndex - 1]?.id,
  };
}

export function activeProjectFileTabSlot(
  state: ProjectFileTabsState,
): ProjectFileTabSlot | undefined {
  return state.tabs.find((tab) => tab.id === state.activeTabId);
}

export function activeProjectFileTab(
  state: ProjectFileTabsState,
): ProjectFileWorkspaceTab | undefined {
  return activeProjectFileTabSlot(state)?.tab;
}
