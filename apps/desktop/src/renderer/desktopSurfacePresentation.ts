import { type ComposerBarMetaItem } from "@awaken/design-system";
import { type ExtensionSection } from "./ExtensionsView";

export type WorkbenchInspectorPresentation = {
  available: boolean;
  effectiveCollapsed: boolean;
  rootClassName:
    | "desktop-root-inspector-expanded"
    | "desktop-root-inspector-collapsed"
    | "desktop-root-inspector-hidden";
};

export type ExpandedWorkspaceSurface = "conversation" | "file";

export function shouldUseCenterComposerWithFileWorkspace({
  standardCenterComposer,
  fileWorkspaceOpen,
  fileWorkspaceExpanded,
  expandedWorkspaceSurface,
}: {
  standardCenterComposer: boolean;
  fileWorkspaceOpen: boolean;
  fileWorkspaceExpanded: boolean;
  expandedWorkspaceSurface: ExpandedWorkspaceSurface;
}): boolean {
  return (
    !(
      fileWorkspaceOpen &&
      fileWorkspaceExpanded &&
      expandedWorkspaceSurface === "file"
    ) && standardCenterComposer
  );
}

export type FileWorkspacePanelAction = "expand" | "contract";

export function fileWorkspacePanelActionForState({
  fileWorkspaceOpen,
  fileWorkspaceExpanded,
  inspectorCollapsed,
}: {
  fileWorkspaceOpen: boolean;
  fileWorkspaceExpanded: boolean;
  inspectorCollapsed: boolean;
}): FileWorkspacePanelAction | undefined {
  if (!fileWorkspaceOpen || inspectorCollapsed) return undefined;
  return fileWorkspaceExpanded ? "contract" : "expand";
}

export function shouldEnableInspectorResizeHandle({
  inspectorAvailable,
  fileWorkspaceExpanded,
}: {
  inspectorAvailable: boolean;
  fileWorkspaceExpanded: boolean;
}): boolean {
  return inspectorAvailable && !fileWorkspaceExpanded;
}

export function composerMetaItemsForFileWorkspaceSurface(
  metaItems: ComposerBarMetaItem[],
  fileWorkspaceSurfaceActive: boolean,
): ComposerBarMetaItem[] {
  return fileWorkspaceSurfaceActive
    ? metaItems.filter((item) => item.id === "model")
    : metaItems;
}

export type WorkbenchNavSection = ExtensionSection | "scheduled-tasks";

/** 一级导航中占据主内容区、且不属于 workbench 的页面（inspector 在这些页面隐藏）。 */
export function workbenchNavSectionForNavId(
  activeNavId: string | undefined,
): WorkbenchNavSection | undefined {
  if (
    activeNavId === "connectors" ||
    activeNavId === "skills" ||
    activeNavId === "plugins"
  ) {
    return activeNavId;
  }
  if (activeNavId === "scheduled-tasks") {
    return "scheduled-tasks";
  }
  return undefined;
}

export function workbenchInspectorPresentation(
  activeNavSection: WorkbenchNavSection | undefined,
  inspectorCollapsed: boolean,
): WorkbenchInspectorPresentation {
  if (activeNavSection) {
    return {
      available: false,
      effectiveCollapsed: true,
      rootClassName: "desktop-root-inspector-hidden",
    };
  }

  return {
    available: true,
    effectiveCollapsed: inspectorCollapsed,
    rootClassName: inspectorCollapsed
      ? "desktop-root-inspector-collapsed"
      : "desktop-root-inspector-expanded",
  };
}
