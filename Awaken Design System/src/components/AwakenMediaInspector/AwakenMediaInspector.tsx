import "./AwakenMediaInspector.css";
import { AwakenIcon } from "../AwakenIcon";
import type { SyntheticEvent } from "react";
import {
  awakenMediaMetadataLabel,
  type AwakenMediaMetadata,
} from "../AwakenMediaItemCard";

export type AwakenMediaInspectorItem = {
  id: string;
  kind: "image" | "video";
  title: string;
  src?: string;
  alt?: string;
  metadata?: AwakenMediaMetadata;
  formatLabel?: string;
};

export type AwakenMediaInspectorProps = {
  item: AwakenMediaInspectorItem;
  sourceUnavailable?: boolean;
  openingExternal?: boolean;
  localOpenLabel?: string;
  revealLabel?: string;
  onClose: () => void;
  onMediaError?: () => void;
  onMetadata?: (metadata: AwakenMediaMetadata) => void;
  onOpenExternal?: () => void;
  onReveal?: () => void;
};

export function AwakenMediaInspector({
  item,
  sourceUnavailable = false,
  openingExternal = false,
  localOpenLabel = "使用本地应用打开",
  revealLabel = "在 Finder/资源管理器中显示",
  onClose,
  onMediaError,
  onMetadata,
  onOpenExternal,
  onReveal,
}: AwakenMediaInspectorProps) {
  const metadataLabel = awakenMediaMetadataLabel(item.metadata, item.formatLabel);
  const unavailableLabel = item.kind === "video" ? "视频预览不可用" : "图片预览不可用";
  const loadingLabel = item.kind === "video" ? "正在载入视频…" : "正在载入图片…";

  const content = sourceUnavailable ? (
    <div className="aw-media-inspector__state" role="alert">
      <span>{unavailableLabel}</span>
      {onMediaError ? (
        <button type="button" onClick={onMediaError}>
          <AwakenIcon concept="rotate-cw" size={16} />
          重新加载
        </button>
      ) : null}
    </div>
  ) : !item.src ? (
    <div className="aw-media-inspector__state" role="status">{loadingLabel}</div>
  ) : item.kind === "image" ? (
    <img
      src={item.src}
      alt={item.alt || item.title}
      draggable={false}
      onLoad={(event: SyntheticEvent<HTMLImageElement>) => {
        const image = event.currentTarget;
        onMetadata?.({ width: image.naturalWidth, height: image.naturalHeight });
      }}
      onError={onMediaError}
    />
  ) : (
    <video
      src={item.src}
      controls
      playsInline
      preload="metadata"
      aria-label={item.alt || item.title}
      onLoadedMetadata={(event) => {
        const video = event.currentTarget;
        onMetadata?.({
          width: video.videoWidth,
          height: video.videoHeight,
          durationSeconds: Number.isFinite(video.duration) ? video.duration : undefined,
        });
      }}
      onError={onMediaError}
    />
  );

  return (
    <section className="aw-media-inspector" aria-label={`媒体查看器：${item.title}`}>
      <header className="aw-media-inspector__header">
        <span>
          <strong title={item.title}>{item.title}</strong>
          {metadataLabel ? <small>{metadataLabel}</small> : null}
        </span>
        <button type="button" aria-label="关闭媒体查看器" onClick={onClose}>
          <AwakenIcon concept="close" size={16} />
        </button>
      </header>
      <div className="aw-media-inspector__canvas">{content}</div>
      {onOpenExternal || onReveal ? (
        <footer className="aw-media-inspector__actions">
          {onOpenExternal ? (
            <button
              type="button"
              disabled={openingExternal}
              aria-label={localOpenLabel}
              onClick={onOpenExternal}
            >
              <AwakenIcon concept="external-open" size={16} />
              打开
            </button>
          ) : null}
          {onReveal ? (
            <button type="button" aria-label={revealLabel} onClick={onReveal}>
              <AwakenIcon concept="folder-expanded" size={16} />
              显示位置
            </button>
          ) : null}
        </footer>
      ) : null}
    </section>
  );
}
