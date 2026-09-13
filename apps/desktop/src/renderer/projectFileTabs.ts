import type { ProjectFileWorkspaceMode } from "./file-preview/FilePreviewHost";

export type ProjectFileWorkspaceTab = {
  id: string;
  projectId: string;
  folderPath: string;
  name: string;
  relativePath: string;
  mode: ProjectFileWorkspaceMode;
  lineRequest?: { id: string; startLine: number; endLine?: number };
};

export function projectFileTabId(
  projectId: string,
  folderPath: string,
  relativePath: string,
): string {
  return JSON.stringify([projectId, folderPath, relativePath]);
}
