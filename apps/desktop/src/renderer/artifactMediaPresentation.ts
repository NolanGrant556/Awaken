import {
  type AwakenMediaPreviewItem,
  type PlatformKind,
} from "@awaken/design-system";
import type { ArtifactOpenRequest } from "../shared/runtime";

export type MediaArtifactPreviewItem = Omit<
  AwakenMediaPreviewItem,
  "src" | "metadata"
> & {
  sourceKey: string;
  request: ArtifactOpenRequest;
};

export type MediaArtifactPreviewState = {
  items: MediaArtifactPreviewItem[];
  activeId: string;
};

export type MediaArtifactRecordRef = {
  id: string;
  path?: string;
};

export function mediaArtifactRequestForPath(
  threadId: string,
  runId: string,
  artifactPath: string,
  artifact?: MediaArtifactRecordRef,
): ArtifactOpenRequest {
  if (artifact) {
    return { threadId, runId, artifactId: artifact.id };
  }
  return {
    threadId,
    runId,
    artifactId: `legacy-media:${artifactPath}`,
    artifactPath,
  };
}

export function artifactMediaSourceKey(request: ArtifactOpenRequest): string {
  return `${request.threadId}:${request.runId}:${request.artifactId}`;
}

export function mediaFormatLabelForPath(path?: string): string | undefined {
  const filename = path?.trim().split(/[\\/]/).pop();
  const extension = filename?.includes(".")
    ? filename.split(".").pop()
    : undefined;
  return extension?.trim().toUpperCase() || undefined;
}

export function artifactPreviewTabMediaIconConcept(
  mediaKind?: "image" | "video",
): "media-image" | "media-video" | undefined {
  if (mediaKind === "image") return "media-image";
  if (mediaKind === "video") return "media-video";
  return undefined;
}

export const MAX_ARTIFACT_MEDIA_SOURCE_RETRIES = 2;

export function mediaRevealLabelForPlatform(platform: PlatformKind): string {
  if (platform === "macos") return "在 Finder 中显示";
  if (platform === "windows") return "在文件资源管理器中显示";
  return "在文件管理器中显示";
}
