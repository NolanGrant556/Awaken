import type { RunNarrativeItem } from "./run-narrative";
import { useEffect, useMemo, useRef, useState } from "react";
import { type AwakenMediaMetadata } from "@awaken/design-system";
import type { ArtifactOpenRequest } from "../shared/runtime";
import {
  mediaArtifactRequestForPath,
  artifactMediaSourceKey,
  MAX_ARTIFACT_MEDIA_SOURCE_RETRIES,
} from "./artifactMediaPresentation";
import type { RuntimeClient } from "./runtime/client";
import type { RuntimeViewState } from "./runtime/reducer";

export function useArtifactMediaSources(
  runtimeClient: RuntimeClient,
  runtimeState: Pick<
    RuntimeViewState,
    "activeThreadId" | "threadId" | "artifacts"
  >,
  visibleNarrativeItems: RunNarrativeItem[],
  workbenchMediaSurfaceActive: boolean,
  setNotice: (message: string | null) => void,
) {
  const [openingArtifactId, setOpeningArtifactId] = useState<string | null>(
    null,
  );

  const [artifactMediaSources, setArtifactMediaSources] = useState<
    Record<string, string | null>
  >({});

  const [artifactMediaMetadata, setArtifactMediaMetadata] = useState<
    Record<string, AwakenMediaMetadata>
  >({});

  const [artifactMediaHydrationEpoch, setArtifactMediaHydrationEpoch] =
    useState(0);

  const artifactMediaSourcesRef = useRef<Record<string, string | null>>({});

  const artifactMediaSourcePendingRef = useRef<Set<string>>(new Set());

  const artifactMediaSourceGenerationRef = useRef(0);

  const artifactMediaSourceRetryRef = useRef<Map<string, number>>(new Map());

  const workbenchMediaSurfaceActiveRef = useRef(true);

  const artifactMediaRequests = useMemo(() => {
    const threadId = runtimeState.activeThreadId ?? runtimeState.threadId;
    if (!threadId) return [];
    const byPath = new Map(
      runtimeState.artifacts
        .filter((artifact) => artifact.path)
        .map((artifact) => [
          artifact.path!.replace(/\\/g, "/").toLowerCase(),
          artifact,
        ]),
    );
    const requests = new Map<
      string,
      { threadId: string; runId: string; artifactId: string }
    >();
    for (const item of visibleNarrativeItems) {
      if (
        item.kind === "mediaGeneration" &&
        (item.status === "ready" || item.status === "partial") &&
        item.runId
      ) {
        const readyPaths = item.mediaItems?.length
          ? item.mediaItems.flatMap((mediaItem) =>
              mediaItem.status === "ready" && mediaItem.path
                ? [mediaItem.path]
                : [],
            )
          : item.artifactPaths;
        for (const artifactPath of readyPaths) {
          const artifact = byPath.get(
            artifactPath.replace(/\\/g, "/").toLowerCase(),
          );
          const request = mediaArtifactRequestForPath(
            threadId,
            item.runId,
            artifactPath,
            artifact,
          );
          requests.set(artifactMediaSourceKey(request), request);
        }
      }
      if (item.kind === "deliverables") {
        for (const file of item.files) {
          if (file.fileType !== "image" && file.fileType !== "video") continue;
          const key = `${threadId}:${file.runId}:${file.artifactId}`;
          requests.set(key, {
            threadId,
            runId: file.runId,
            artifactId: file.artifactId,
          });
        }
      }
    }
    return [...requests.entries()].map(([key, request]) => ({ key, request }));
  }, [
    runtimeState.activeThreadId,
    runtimeState.artifacts,
    runtimeState.threadId,
    visibleNarrativeItems,
  ]);

  useEffect(() => {
    artifactMediaSourceGenerationRef.current += 1;
    artifactMediaSourcePendingRef.current.clear();
    artifactMediaSourceRetryRef.current.clear();
    artifactMediaSourcesRef.current = {};
    setArtifactMediaSources({});
    setArtifactMediaMetadata({});
  }, [runtimeState.activeThreadId]);

  useEffect(() => {
    const sourceGeneration = artifactMediaSourceGenerationRef.current;
    for (const { key, request } of artifactMediaRequests) {
      if (
        Object.prototype.hasOwnProperty.call(
          artifactMediaSourcesRef.current,
          key,
        )
      )
        continue;
      if (artifactMediaSourcePendingRef.current.has(key)) continue;
      artifactMediaSourcePendingRef.current.add(key);
      void runtimeClient
        .getArtifactMediaSource(request)
        .then((result) => {
          artifactMediaSourcePendingRef.current.delete(key);
          if (sourceGeneration !== artifactMediaSourceGenerationRef.current)
            return;
          const nextSource = result.ok ? result.src : null;
          if (result.ok) {
            artifactMediaSourceRetryRef.current.delete(key);
          }
          artifactMediaSourcesRef.current = {
            ...artifactMediaSourcesRef.current,
            [key]: nextSource,
          };
          setArtifactMediaSources((current) => ({
            ...current,
            [key]: nextSource,
          }));
        })
        .catch(() => {
          artifactMediaSourcePendingRef.current.delete(key);
          if (sourceGeneration !== artifactMediaSourceGenerationRef.current)
            return;
          artifactMediaSourcesRef.current = {
            ...artifactMediaSourcesRef.current,
            [key]: null,
          };
          setArtifactMediaSources((current) => ({ ...current, [key]: null }));
        });
    }
  }, [artifactMediaHydrationEpoch, artifactMediaRequests, runtimeClient]);

  useEffect(() => {
    const wasActive = workbenchMediaSurfaceActiveRef.current;
    workbenchMediaSurfaceActiveRef.current = workbenchMediaSurfaceActive;
    if (!workbenchMediaSurfaceActive || wasActive) return;
    // Skills/Connectors/Scheduled Tasks unmount Workbench; reissue short-lived opaque sources on return.
    artifactMediaSourcePendingRef.current.clear();
    artifactMediaSourceRetryRef.current.clear();
    artifactMediaSourcesRef.current = {};
    setArtifactMediaSources({});
    setArtifactMediaHydrationEpoch((value) => value + 1);
  }, [workbenchMediaSurfaceActive]);

  function recordArtifactMediaMetadata(
    sourceKey: string,
    metadata: AwakenMediaMetadata,
  ): void {
    setArtifactMediaMetadata((current) => {
      const previous = current[sourceKey];
      if (
        previous?.width === metadata.width &&
        previous.height === metadata.height &&
        previous.durationSeconds === metadata.durationSeconds
      ) {
        return current;
      }
      return { ...current, [sourceKey]: metadata };
    });
  }

  function refreshArtifactMediaSource(
    key: string,
  ): void {
    const retries = artifactMediaSourceRetryRef.current.get(key) ?? 0;
    if (retries >= MAX_ARTIFACT_MEDIA_SOURCE_RETRIES) {
      artifactMediaSourcesRef.current = {
        ...artifactMediaSourcesRef.current,
        [key]: null,
      };
      setArtifactMediaSources((current) => ({ ...current, [key]: null }));
      return;
    }
    artifactMediaSourceRetryRef.current.set(key, retries + 1);
    artifactMediaSourcePendingRef.current.delete(key);
    const nextSources = { ...artifactMediaSourcesRef.current };
    delete nextSources[key];
    artifactMediaSourcesRef.current = nextSources;
    setArtifactMediaSources(nextSources);
    setArtifactMediaHydrationEpoch((value) => value + 1);
  }

  async function openArtifactExternally(
    request: ArtifactOpenRequest,
  ): Promise<void> {
    setOpeningArtifactId(request.artifactId);
    try {
      const result = await runtimeClient.openArtifact(request);
      if (!result.ok) setNotice(result.message);
    } finally {
      setOpeningArtifactId((current) =>
        current === request.artifactId ? null : current,
      );
    }
  }

  async function revealArtifact(request: ArtifactOpenRequest): Promise<void> {
    const result = await runtimeClient.revealArtifact(request);
    if (!result.ok) setNotice(result.message);
  }
  return {
    artifactMediaSources,
    artifactMediaMetadata,
    openingArtifactId,
    recordArtifactMediaMetadata,
    refreshArtifactMediaSource,
    openArtifactExternally,
    revealArtifact,
  };
}
