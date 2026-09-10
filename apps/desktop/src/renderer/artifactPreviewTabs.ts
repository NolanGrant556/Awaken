import type { ArtifactPreviewSelection } from "./file-preview/ArtifactPreviewHost";

export type ArtifactPreviewTab = {
  id: string;
  selection: ArtifactPreviewSelection;
};

export function artifactPreviewTabId(
  selection: Pick<ArtifactPreviewSelection, "threadId" | "runId" | "artifactId">,
): string {
  return JSON.stringify([
    selection.threadId,
    selection.runId,
    selection.artifactId,
  ]);
}
