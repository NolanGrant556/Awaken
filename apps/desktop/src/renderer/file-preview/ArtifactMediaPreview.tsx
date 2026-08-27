import { useEffect, useMemo, useState, type SyntheticEvent } from "react";
import {
  AwakenIcon,
  awakenMediaMetadataLabel,
  type AwakenMediaMetadata,
} from "@awaken/design-system";
import type {
  ArtifactOpenRequest,
  ProjectFilePreviewUrlHandle,
} from "../../shared/runtime";
import type { RuntimeClient } from "../runtime/client";
import ImagePreview from "./ImagePreview";
import PreviewErrorBoundary from "./PreviewErrorBoundary";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";

export type ArtifactMediaPreviewProps = {
  runtimeClient: Pick<RuntimeClient, "openArtifact" | "revealArtifact">;
  request: ArtifactOpenRequest;
  title: string;
  kind: "image" | "video";
  src?: string;
  sourceUnavailable?: boolean;
  metadata?: AwakenMediaMetadata;
  revealLabel?: string;
  onMediaError?: () => void;
  onMetadata?: (metadata: AwakenMediaMetadata) => void;
};

function artifactMediaIdentity(request: ArtifactOpenRequest): string {
  return `${request.threadId}:${request.runId}:${request.artifactId}`;
}

function formatLabelForTitle(title: string): string | undefined {
  const filename = title.trim().split(/[\\/]/).pop();
  const extension = filename?.includes(".") ? filename.split(".").pop() : undefined;
  return extension?.trim().toUpperCase() || undefined;
}

function ArtifactVideoPreview({
  src,
  title,
  metadata,
  frameProps,
  onMediaError,
  onMetadata,
}: {
  src: string;
  title: string;
  metadata?: AwakenMediaMetadata;
  frameProps: Omit<PreviewFrameProps, "children" | "formatActions" | "formatLabel">;
  onMediaError: () => void;
  onMetadata?: (metadata: AwakenMediaMetadata) => void;
}) {
  const metadataLabel = awakenMediaMetadataLabel(metadata, formatLabelForTitle(title));
  return (
    <PreviewFrame
      {...frameProps}
      formatLabel="视频"
      formatActions={
        metadataLabel ? (
          <output
            className="project-file-preview-heading__value project-file-video-preview__metadata"
            aria-label="视频信息"
          >
            {metadataLabel}
          </output>
        ) : undefined
      }
    >
      <div className="project-file-video-preview__canvas">
        <video
          src={src}
          controls
          playsInline
          preload="metadata"
          aria-label={title}
          onLoadedMetadata={(event: SyntheticEvent<HTMLVideoElement>) => {
            const video = event.currentTarget;
            if (video.videoWidth <= 0 || video.videoHeight <= 0) return;
            onMetadata?.({
              width: video.videoWidth,
              height: video.videoHeight,
              durationSeconds: Number.isFinite(video.duration) ? video.duration : undefined,
            });
          }}
          onError={onMediaError}
        />
      </div>
    </PreviewFrame>
  );
}

export default function ArtifactMediaPreview({
  runtimeClient,
  request,
  title,
  kind,
  src,
  sourceUnavailable = false,
  metadata,
  revealLabel = "在文件管理器中显示",
  onMediaError,
  onMetadata,
}: ArtifactMediaPreviewProps) {
  const identity = artifactMediaIdentity(request);
  const [actionError, setActionError] = useState<string>();

  useEffect(() => {
    setActionError(undefined);
  }, [identity, src]);

  function runArtifactAction(
    action: (request: ArtifactOpenRequest) => Promise<{ ok: boolean; message?: string }>,
  ): void {
    void action(request)
      .then((result) => {
        if (result.ok) return;
        setActionError(result.message ?? "操作暂时无法完成。");
      })
      .catch(() => setActionError("操作暂时无法完成。"));
  }

  const frameProps = useMemo<
    Omit<PreviewFrameProps, "children" | "formatActions" | "formatLabel">
  >(
    () => ({
      displayName: title,
      onReload: () => onMediaError?.(),
      onOpenExternal: () => runArtifactAction(runtimeClient.openArtifact),
      onReveal: () => runArtifactAction(runtimeClient.revealArtifact),
      revealLabel,
    }),
    [onMediaError, request, revealLabel, runtimeClient, title],
  );

  if (actionError || sourceUnavailable) {
    return (
      <div className="project-file-preview-state is-error" role="alert">
        <strong>无法预览此交付物</strong>
        <span>{actionError ?? "媒体预览不可用，请重新载入。"}</span>
        <div className="project-file-preview-state__actions">
          {onMediaError ? (
            <button
              type="button"
              onClick={() => {
                setActionError(undefined);
                onMediaError();
              }}
            >
              <AwakenIcon concept="refresh" size={16} />
              重试
            </button>
          ) : null}
          <button
            type="button"
            onClick={() => runArtifactAction(runtimeClient.openArtifact)}
          >
            <AwakenIcon concept="external-open" size={16} />
            使用系统默认应用打开
          </button>
          <button
            type="button"
            onClick={() => runArtifactAction(runtimeClient.revealArtifact)}
          >
            <AwakenIcon concept="locate-folder" size={16} />
            {revealLabel}
          </button>
        </div>
      </div>
    );
  }

  if (!src) {
    return (
      <div className="project-file-preview-state" role="status">
        <span className="project-file-preview-spinner" aria-hidden="true" />
        <strong>{title}</strong>
        <span>正在准备媒体预览…</span>
      </div>
    );
  }

  const handleMediaError = (): void => {
    if (onMediaError) {
      onMediaError();
      return;
    }
    setActionError(kind === "video" ? "视频加载失败。" : "图片加载失败。");
  };

  const imagePreview: ProjectFilePreviewUrlHandle = {
    previewId: identity,
    requestId: identity,
    kind: "image",
    transport: "url",
    displayName: title,
    relativePath: title,
    byteLength: 0,
    src,
  };

  return (
    <PreviewErrorBoundary
      resetKey={`${identity}:${src}`}
      onError={() => setActionError("预览组件意外停止运行，请重试。")}
    >
      {kind === "image" ? (
        <ImagePreview
          preview={imagePreview}
          frameProps={frameProps}
          onLoadError={handleMediaError}
        />
      ) : (
        <ArtifactVideoPreview
          src={src}
          title={title}
          metadata={metadata}
          frameProps={frameProps}
          onMediaError={handleMediaError}
          onMetadata={onMetadata}
        />
      )}
    </PreviewErrorBoundary>
  );
}
