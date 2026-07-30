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
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type SyntheticEvent,
} from "react";

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

export type AwakenMediaItemCardProps = {
  kind: AwakenMediaItemKind;
  status: AwakenMediaItemStatus;
  title: string;
  detail?: string;
  src?: string;
  alt?: string;
  index?: number;
  aspectRatio?: number;
  disabled?: boolean;
  openingExternal?: boolean;
  onPreview?: () => void;
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
  disabled = false,
  openingExternal = false,
  onPreview,
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
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDetailsElement>(null);
  const ready = status === "ready";
  const transient =
    !ready && status !== "unknown" && status !== "error" && status !== "cancelled";
  const previewEnabled = ready && loaded && Boolean(onPreview) && !disabled;
  const menuAvailable = Boolean(onOpenExternal || onReveal);
  const fallbackRatio = kind === "video" ? 16 / 9 : 4 / 3;
  const cardStyle = {
    "--aw-media-item-aspect": resolvedRatio ?? positiveRatio(aspectRatio) ?? fallbackRatio,
  } as CSSProperties;

  useEffect(() => {
    setLoaded(false);
    setResolvedRatio(positiveRatio(aspectRatio));
  }, [aspectRatio, kind, src]);

  useEffect(() => {
    if (!menuOpen) return;
    const closeOnOutsideInteraction = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenuOpen(false);
    };
    document.addEventListener("pointerdown", closeOnOutsideInteraction);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsideInteraction);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [menuOpen]);

  const handleImageLoad = (event: SyntheticEvent<HTMLImageElement>) => {
    const image = event.currentTarget;
    if (image.naturalWidth > 0 && image.naturalHeight > 0) {
      setResolvedRatio(image.naturalWidth / image.naturalHeight);
    }
    setLoaded(true);
  };

  const handleVideoMetadata = (event: SyntheticEvent<HTMLVideoElement>) => {
    const video = event.currentTarget;
    if (video.videoWidth > 0 && video.videoHeight > 0) {
      setResolvedRatio(video.videoWidth / video.videoHeight);
    }
    setLoaded(true);
  };

  const media = (
    <>
      {ready && src && kind === "image" ? (
        <img
          className="aw-media-item-card-media"
          src={src}
          alt={alt || title}
          draggable={false}
          onLoad={handleImageLoad}
          onError={() => setLoaded(false)}
        />
      ) : null}
      {ready && src && kind === "video" ? (
        <video
          className="aw-media-item-card-media"
          src={src}
          muted
          playsInline
          preload="metadata"
          aria-label={alt || title}
          onLoadedMetadata={handleVideoMetadata}
          onError={() => setLoaded(false)}
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
          <span>{detail || statusLabel[status]}</span>
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
          <details
            ref={menuRef}
            className="aw-media-item-card-menu"
            open={menuOpen}
            onToggle={(event) => setMenuOpen(event.currentTarget.open)}
          >
            <summary aria-label={`更多 ${title} 操作`} aria-expanded={menuOpen}>
              <MoreHorizontal aria-hidden="true" size={17} strokeWidth={1.8} />
            </summary>
            <div className="aw-media-item-card-menu-popover" role="menu">
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
          </details>
        ) : null}
      </div>
    </article>
  );
}
