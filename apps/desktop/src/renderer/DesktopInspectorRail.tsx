import { AwakenInspectorRail as InspectorRail } from "@awaken/design-system";
import { INSPECTOR_FILE_ITEMS } from "./desktopLayoutConstants";
import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";

export function DesktopInspectorRail({ handleOpenFileWorkspace }: {
  handleOpenFileWorkspace: (mode?: ProjectFileWorkspaceMode) => void;
}) {
  return <InspectorRail
    ariaLabel="文件"
    items={INSPECTOR_FILE_ITEMS}
    onSelect={id => handleOpenFileWorkspace(id === "browser" ? "browser" : "files")}
  />;
}
