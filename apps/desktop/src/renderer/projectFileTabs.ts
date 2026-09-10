import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";

export type ProjectFileWorkspaceTab = {
  id: string;
  projectId: string;
  folderPath: string;
  name: string;
  relativePath: string;
  mode: ProjectFileWorkspaceMode;
};

export function projectFileTabId(
  projectId: string,
  folderPath: string,
  relativePath: string,
): string {
  return JSON.stringify([projectId, folderPath, relativePath]);
}
