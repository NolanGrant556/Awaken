import "./AwakenMediaItemCard.css";
import {
  AlertCircle,
  Image as ImageIcon,
  LoaderCircle,
  MoreHorizontal,
  Play,
  Video,
} from "lucide-react";
import {
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type SyntheticEvent,
} from "react";
import { AwakenPopover } from "../AwakenPopover";

export type AwakenMediaItemKind = "image" | "video";

export type AwakenMediaItemStatus =
  | "preparing"
  | "queued"
  | "generating"
  | "downloading"
  | "ready"
  | "unknown"
  | "error"
  | "cancelled";

export type AwakenMediaMetadata = {
  width: number;
  height: number;
  durationSeconds?: number;
};

export function awakenMediaMetadataLabel(
  metadata?: AwakenMediaMetadata,
  formatLabel?: string,
): string | undefined {
  const parts: string[] = [];
  if (metadata && metadata.width > 0 && metadata.height > 0) {
    parts.push(`${Math.round(metadata.width)}×${Math.round(metadata.height)}`);
    if (
      typeof metadata.durationSeconds === "number" &&
      Number.isFinite(metadata.durationSeconds) &&
      metadata.durationSeconds >= 0
    ) {
      const duration = metadata.durationSeconds;
      parts.push(duration < 10 ? `${duration.toFixed(1)} 秒` : `${Math.round(duration)} 秒`);
    }
  }
  const normalizedFormat = formatLabel?.trim().replace(/^\./, "").toUpperCase();
  if (normalizedFormat) parts.push(normalizedFormat);
  return parts.length > 0 ? parts.join(" · ") : undefined;
}

export type AwakenMediaItemCardProps = {
  kind: AwakenMediaItemKind;
  status: AwakenMediaItemStatus;
  title: string;
  detail?: string;
  src?: string;
  alt?: string;
  index?: number;
  aspectRatio?: number;
  metadata?: AwakenMediaMetadata;
  formatLabel?: string;
  disabled?: boolean;
  openingExternal?: boolean;
  onPreview?: () => void;
  /** Fired when the current media element fails to load; consumers may reissue a source. */
  onMediaError?: () => void;
  /** Reports intrinsic dimensions (and video duration) after metadata loads. */
  onMetadata?: (metadata: AwakenMediaMetadata) => void;
  /** Optional secondary action that pins this media in the Desktop inspector. */
  onViewInInspector?: () => void;
  onOpenExternal?: () => void;
  onReveal?: () => void;
  localOpenLabel?: string;
  revealLabel?: string;
  className?: string;
};

const LOADING_CELL_COUNT = 9;
const LOADING_CELLS = Array.from({ length: LOADING_CELL_COUNT }, (_, index) => index);

const statusLabel: Record<AwakenMediaItemStatus, string> = {
  preparing: "正在准备",
  queued: "等待生成",
  generating: "正在生成",
  downloading: "正在载入",
  ready: "已生成",
  unknown: "状态待确认",
  error: "生成失败",
  cancelled: "已停止",
};

function loadingCellStyle(index: number): CSSProperties {
  return {
    "--aw-media-item-delay": `${index * 70 - 560}ms`,
  } as CSSProperties;
}

function positiveRatio(value?: number): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;
}

export function AwakenMediaItemCard({
  kind,
  status,
  title,
  detail,
  src,
  alt,
  index,
  aspectRatio,
  metadata,
  formatLabel,
  disabled = false,
  openingExternal = false,
  onPreview,
  onMediaError,
  onMetadata,
  onViewInInspector,
  onOpenExternal,
  onReveal,
  localOpenLabel = "使用本地应用打开",
  revealLabel = "在 Finder/资源管理器中显示",
  className,
}: AwakenMediaItemCardProps) {
  const [loaded, setLoaded] = useState(false);
  const [resolvedRatio, setResolvedRatio] = useState<number | undefined>(() =>
    positiveRatio(aspectRatio),
  );
  const [resolvedMetadata, setResolvedMetadata] = useState<AwakenMediaMetadata | undefined>(
    metadata,
  );
  const [menuOpen, setMenuOpen] = useState(false);
  const imageRef = useRef<HTMLImageElement | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const ready = status === "ready";
  const transient =
    !ready && status !== "unknown" && status !== "error" && status !== "cancelled";
  const previewEnabled = ready && loaded && Boolean(onPreview) && !disabled;
  const menuAvailable = Boolean(onViewInInspector || onOpenExternal || onReveal);
  const fallbackRatio = kind === "video" ? 16 / 9 : 4 / 3;
  const cardStyle = {
    "--aw-media-item-aspect": resolvedRatio ?? positiveRatio(aspectRatio) ?? fallbackRatio,
  } as CSSProperties;
  const resolvedDetail =
    (resolvedMetadata
      ? awakenMediaMetadataLabel(resolvedMetadata, formatLabel)
      : undefined) || detail || statusLabel[status];

  const applyMetadata = (nextMetadata: AwakenMediaMetadata) => {
    setResolvedMetadata(nextMetadata);
    onMetadata?.(nextMetadata);
  };

  useLayoutEffect(() => {
    setLoaded(false);
    setResolvedRatio(positiveRatio(aspectRatio));
    setResolvedMetadata(metadata);
    if (!ready || !src) return;
    if (kind === "image") {
      const image = imageRef.current;
      if (!image?.complete || image.naturalWidth <= 0) return;
      if (image.naturalHeight > 0) {
        setResolvedRatio(image.naturalWidth / image.naturalHeight);
        applyMetadata({ width: image.naturalWidth, height: image.naturalHeight });
      }
      setLoaded(true);
      return;
    }
    const video = videoRef.current;
    if (!video || video.readyState < HTMLMediaElement.HAVE_METADATA) return;
    if (video.videoWidth > 0 && video.videoHeight > 0) {
      setResolvedRatio(video.videoWidth / video.videoHeight);
      applyMetadata({
        width: video.videoWidth,
        height: video.videoHeight,
        durationSeconds: Number.isFinite(video.duration) ? video.duration : undefined,
      });
    }
    setLoaded(true);
  }, [aspectRatio, kind, metadata, ready, src]);

  const handleImageLoad = (event: SyntheticEvent<HTMLImageElement>) => {
    const image = event.currentTarget;
    if (image.naturalWidth > 0 && image.naturalHeight > 0) {
      setResolvedRatio(image.naturalWidth / image.naturalHeight);
      applyMetadata({ width: image.naturalWidth, height: image.naturalHeight });
    }
    setLoaded(true);
  };

  const handleVideoMetadata = (event: SyntheticEvent<HTMLVideoElement>) => {
    const video = event.currentTarget;
    if (video.videoWidth > 0 && video.videoHeight > 0) {
      setResolvedRatio(video.videoWidth / video.videoHeight);
      applyMetadata({
        width: video.videoWidth,
        height: video.videoHeight,
        durationSeconds: Number.isFinite(video.duration) ? video.duration : undefined,
      });
    }
    setLoaded(true);
  };

  const handleMediaError = () => {
    setLoaded(false);
    onMediaError?.();
  };

  const media = (
    <>
      {ready && src && kind === "image" ? (
        <img
          ref={imageRef}
          className="aw-media-item-card-media"
          src={src}
          alt={alt || title}
          draggable={false}
          onLoad={handleImageLoad}
          onError={handleMediaError}
        />
      ) : null}
      {ready && src && kind === "video" ? (
        <video
          ref={videoRef}
          className="aw-media-item-card-media"
          src={src}
          muted
          playsInline
          preload="metadata"
          aria-label={alt || title}
          onLoadedMetadata={handleVideoMetadata}
          onError={handleMediaError}
        />
      ) : null}
      {!loaded || !ready ? (
        <span className="aw-media-item-card-loading" aria-hidden="true">
          <span className="aw-media-item-card-loading-grid">
            {LOADING_CELLS.map((cellIndex) => (
              <span key={cellIndex} style={loadingCellStyle(cellIndex)} />
            ))}
          </span>
        </span>
      ) : null}
      {status === "unknown" || status === "error" || status === "cancelled" ? (
        <span className="aw-media-item-card-terminal" aria-hidden="true">
          <AlertCircle size={20} strokeWidth={1.8} />
        </span>
      ) : null}
      {loaded && ready && kind === "video" ? (
        <span className="aw-media-item-card-play" aria-hidden="true">
          <Play size={17} fill="currentColor" strokeWidth={1.6} />
        </span>
      ) : null}
      {typeof index === "number" ? (
        <span className="aw-media-item-card-index" aria-hidden="true">{index}</span>
      ) : null}
    </>
  );

  return (
    <article
      className={["aw-media-item-card", `aw-media-item-card-${kind}`, className]
        .filter(Boolean)
        .join(" ")}
      data-status={status}
      data-loaded={loaded ? "true" : "false"}
      data-menu-open={menuOpen ? "true" : undefined}
      style={cardStyle}
      aria-label={`${title}，${statusLabel[status]}`}
    >
      {onPreview ? (
        <button
          type="button"
          className="aw-media-item-card-surface"
          onClick={onPreview}
          disabled={!previewEnabled}
          aria-label={`预览 ${title}`}
        >
          {media}
        </button>
      ) : (
        <div className="aw-media-item-card-surface">{media}</div>
      )}

      <div className="aw-media-item-card-footer">
        <span className="aw-media-item-card-kind" aria-hidden="true">
          {kind === "image" ? (
            <ImageIcon size={14} strokeWidth={1.8} />
          ) : (
            <Video size={14} strokeWidth={1.8} />
          )}
        </span>
        <span className="aw-media-item-card-copy">
          <strong title={title}>{title}</strong>
          <span title={resolvedDetail}>{resolvedDetail}</span>
        </span>
        {transient ? (
          <LoaderCircle
            className="aw-media-item-card-spinner"
            size={15}
            strokeWidth={1.9}
            aria-hidden="true"
          />
        ) : null}
        {openingExternal ? (
          <LoaderCircle
            className="aw-media-item-card-spinner"
            size={15}
            strokeWidth={1.9}
            aria-hidden="true"
          />
        ) : null}
        {menuAvailable ? (
          <div className="aw-media-item-card-menu">
            <AwakenPopover
              trigger={
                <button
                  type="button"
                  className="aw-media-item-card-menu-trigger"
                  aria-label={`更多 ${title} 操作`}
                  aria-expanded={menuOpen}
                  aria-haspopup="menu"
                >
                  <MoreHorizontal aria-hidden="true" size={17} strokeWidth={1.8} />
                </button>
              }
              content={
                <div className="aw-media-item-card-menu-popover">
                  {onViewInInspector ? (
                    <button
                      type="button"
                      role="menuitem"
                      disabled={disabled}
                      onClick={() => {
                        setMenuOpen(false);
                        onViewInInspector();
                      }}
                    >
                      在右侧栏查看
                    </button>
                  ) : null}
                  {onOpenExternal ? (
                    <button
                      type="button"
                      role="menuitem"
                      disabled={disabled || openingExternal}
                      onClick={() => {
                        setMenuOpen(false);
                        onOpenExternal();
                      }}
                    >
                      {localOpenLabel}
                    </button>
                  ) : null}
                  {onReveal ? (
                    <button
                      type="button"
                      role="menuitem"
                      disabled={disabled}
                      onClick={() => {
                        setMenuOpen(false);
                        onReveal();
                      }}
                    >
                      {revealLabel}
                    </button>
                  ) : null}
                </div>
              }
              open={menuOpen}
              onOpenChange={setMenuOpen}
              placement="bottom-end"
              offset={4}
              role="menu"
              portalled={false}
            />
          </div>
        ) : null}
      </div>
    </article>
  );
}
