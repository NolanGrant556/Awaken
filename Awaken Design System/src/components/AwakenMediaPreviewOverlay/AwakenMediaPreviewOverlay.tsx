import "./AwakenMediaPreviewOverlay.css";
import { AwakenIcon } from "../AwakenIcon";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type SyntheticEvent,
} from "react";
import type { AwakenMediaMetadata } from "../AwakenMediaItemCard";

export type AwakenMediaPreviewItem = {
  id: string;
  kind: "image" | "video";
  title: string;
  src?: string;
  alt?: string;
  metadata?: AwakenMediaMetadata;
  formatLabel?: string;
};

export type AwakenMediaPreviewOverlayProps = {
  item: AwakenMediaPreviewItem;
  position: number;
  count: number;
  onClose: () => void;
  onPrevious?: () => void;
  onNext?: () => void;
  onOpenExternal?: () => void;
  onReveal?: () => void;
  openingExternal?: boolean;
  localOpenLabel?: string;
  revealLabel?: string;
  sourceUnavailable?: boolean;
  autoPlayVideo?: boolean;
  onMediaError?: () => void;
  onMetadata?: (metadata: AwakenMediaMetadata) => void;
};

const focusableSelector = [
  "button:not([disabled])",
  "video[controls]",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

export function AwakenMediaPreviewOverlay({
  item,
  position,
  count,
  onClose,
  onPrevious,
  onNext,
  onOpenExternal,
  onReveal,
  openingExternal = false,
  localOpenLabel = "使用本地应用打开",
  revealLabel = "在 Finder/资源管理器中显示",
  sourceUnavailable = false,
  autoPlayVideo = true,
  onMediaError,
  onMetadata,
}: AwakenMediaPreviewOverlayProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const fullscreenRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const autoplayAttemptedSourceRef = useRef<string | undefined>(undefined);
  const dragRef = useRef<{
    pointerId: number;
    x: number;
    y: number;
    scrollLeft: number;
    scrollTop: number;
  } | undefined>(undefined);
  const [scale, setScale] = useState<number>();
  const fit = scale === undefined;
  const [naturalSize, setNaturalSize] = useState<{ width: number; height: number }>();
  const [dragging, setDragging] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [autoplayRejected, setAutoplayRejected] = useState(false);

  useEffect(() => {
    setScale(undefined);
    setNaturalSize(undefined);
    setDragging(false);
    setLoadFailed(false);
    setAutoplayRejected(false);
    // Keep programmatic focus on the outline-free dialog surface. Arrow-key
    // navigation changes the item and the global modality to keyboard; focusing
    // the close button here would therefore paint a misleading blue ring even
    // though the user did not navigate to that button with Tab.
    requestAnimationFrame(() => dialogRef.current?.focus());
  }, [item.id, item.src]);

  const reportMediaError = () => {
    setLoadFailed(true);
    onMediaError?.();
  };

  const attemptVideoPlayback = (video: HTMLVideoElement) => {
    if (!autoPlayVideo || !item.src) return;
    if (autoplayAttemptedSourceRef.current === item.src) return;
    autoplayAttemptedSourceRef.current = item.src;
    void video.play().then(
      () => setAutoplayRejected(false),
      () => setAutoplayRejected(true),
    );
  };

  useEffect(() => {
    const previousActiveElement = document.activeElement;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onClose();
      } else if (event.key === "ArrowLeft" && onPrevious) {
        event.preventDefault();
        onPrevious();
      } else if (event.key === "ArrowRight" && onNext) {
        event.preventDefault();
        onNext();
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      if (previousActiveElement instanceof HTMLElement) previousActiveElement.focus();
    };
  }, [onClose, onNext, onPrevious]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (item.kind !== "image" || !canvas || !naturalSize) return;

    const handleWheel = (event: WheelEvent) => {
      // Chromium delivers trackpad pinch as Ctrl+wheel. A vertical two-finger
      // scroll also zooms here. Horizontal scrolling can still pan the image.
      if (!event.deltaY || (!event.ctrlKey && Math.abs(event.deltaX) > Math.abs(event.deltaY))) return;
      event.preventDefault();
      const fitScale = Math.min(1, canvas.clientWidth / naturalSize.width, canvas.clientHeight / naturalSize.height);
      if (fitScale <= 0) return;
      const unit = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? canvas.clientHeight : 1;
      const multiplier = Math.exp(-event.deltaY * unit * 0.004);
      setScale((current) => {
        const next = (current ?? fitScale) * multiplier;
        return Math.min(8, Math.max(Math.min(0.1, fitScale), next));
      });
    };
    canvas.addEventListener("wheel", handleWheel, { passive: false });
    return () => canvas.removeEventListener("wheel", handleWheel);
  }, [item.kind, naturalSize]);

  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (fit || item.kind !== "image" || !canvasRef.current) return;
    dragRef.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      scrollLeft: canvasRef.current.scrollLeft,
      scrollTop: canvasRef.current.scrollTop,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
    setDragging(true);
  };

  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    const canvas = canvasRef.current;
    if (!drag || drag.pointerId !== event.pointerId || !canvas) return;
    canvas.scrollLeft = drag.scrollLeft - (event.clientX - drag.x);
    canvas.scrollTop = drag.scrollTop - (event.clientY - drag.y);
  };

  const finishPointerDrag = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (dragRef.current?.pointerId !== event.pointerId) return;
    dragRef.current = undefined;
    setDragging(false);
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  };

  const handleDialogKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Tab") return;
    const focusable = Array.from(
      event.currentTarget.querySelectorAll<HTMLElement>(focusableSelector),
    );
    if (focusable.length === 0) {
      event.preventDefault();
      return;
    }
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const imageSizeStyle =
    scale !== undefined && naturalSize
      ? {
          width: `${naturalSize.width * scale}px`,
          height: `${naturalSize.height * scale}px`,
        }
      : undefined;

  return (
    <div
      className="aw-media-preview-overlay"
      role="presentation"
      onPointerDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="aw-media-preview-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        onKeyDown={handleDialogKeyDown}
      >
        <header className="aw-media-preview-header">
          <span className="aw-media-preview-heading">
            <strong id={titleId} title={item.title}>{item.title}</strong>
            {count > 1 ? <span>{position + 1} / {count}</span> : null}
          </span>
          <button
            type="button"
            className="aw-media-preview-icon-button"
            aria-label="关闭媒体预览"
            onClick={onClose}
          >
            <AwakenIcon concept="close" size={20} />
          </button>
        </header>

        <div ref={fullscreenRef} className="aw-media-preview-content">
          {onPrevious ? (
            <button
              type="button"
              className="aw-media-preview-nav aw-media-preview-nav-previous"
              aria-label="查看上一个媒体"
              onClick={onPrevious}
            >
              <AwakenIcon concept="back" size={24} />
            </button>
          ) : null}
          {onNext ? (
            <button
              type="button"
              className="aw-media-preview-nav aw-media-preview-nav-next"
              aria-label="查看下一个媒体"
              onClick={onNext}
            >
              <AwakenIcon concept="disclose" size={24} />
            </button>
          ) : null}

          {item.kind === "image" ? (
            <div
              ref={canvasRef}
              className={`aw-media-preview-image-canvas${fit ? " is-fit" : ""}${dragging ? " is-dragging" : ""}`}
              onPointerDown={handlePointerDown}
              onPointerMove={handlePointerMove}
              onPointerUp={finishPointerDrag}
              onPointerCancel={finishPointerDrag}
            >
              {sourceUnavailable ? (
                <span className="aw-media-preview-error">图片预览加载失败，可以尝试使用本地应用打开。</span>
              ) : !item.src || loadFailed ? (
                <span className="aw-media-preview-error" role="status">正在重新载入图片预览…</span>
              ) : (
                <span className="aw-media-preview-image-stage">
                  <img
                    src={item.src}
                    style={imageSizeStyle}
                    alt={item.alt || item.title}
                    draggable={false}
                    onLoad={(event: SyntheticEvent<HTMLImageElement>) => {
                      const image = event.currentTarget;
                      const metadata = {
                        width: image.naturalWidth,
                        height: image.naturalHeight,
                      };
                      setNaturalSize(metadata);
                      onMetadata?.(metadata);
                    }}
                    onError={reportMediaError}
                  />
                </span>
              )}
            </div>
          ) : (
            <div className="aw-media-preview-video-canvas">
              {sourceUnavailable ? (
                <span className="aw-media-preview-error">视频预览加载失败，可以尝试使用本地应用打开。</span>
              ) : !item.src || loadFailed ? (
                <span className="aw-media-preview-error" role="status">正在重新载入视频预览…</span>
              ) : (
                <>
                  <video
                    ref={videoRef}
                    src={item.src}
                    controls
                    autoPlay={autoPlayVideo}
                    playsInline
                    preload="metadata"
                    aria-label={item.alt || item.title}
                    onLoadedMetadata={(event) => {
                      const video = event.currentTarget;
                      onMetadata?.({
                        width: video.videoWidth,
                        height: video.videoHeight,
                        durationSeconds: Number.isFinite(video.duration)
                          ? video.duration
                          : undefined,
                      });
                      attemptVideoPlayback(video);
                    }}
                    onPlay={() => setAutoplayRejected(false)}
                    onError={reportMediaError}
                  />
                  {autoplayRejected ? (
                    <button
                      type="button"
                      className="aw-media-preview-play-fallback"
                      onClick={() => {
                        const video = videoRef.current;
                        if (!video) return;
                        void video.play().then(
                          () => setAutoplayRejected(false),
                          () => setAutoplayRejected(true),
                        );
                      }}
                    >
                      播放视频
                    </button>
                  ) : null}
                </>
              )}
            </div>
          )}
        </div>

        {item.kind === "video" || onOpenExternal || onReveal ? (
          <footer className="aw-media-preview-toolbar">
            {item.kind === "video" ? (
              <div className="aw-media-preview-toolbar-group aw-media-preview-zoom-controls">
                <button
                  type="button"
                  className="aw-media-preview-compact-button"
                  aria-label="全屏"
                  title="全屏"
                  onClick={() => {
                    void fullscreenRef.current?.requestFullscreen?.();
                  }}
                >
                  <AwakenIcon concept="expand-full" size={16} />
                </button>
              </div>
            ) : null}
            <div className="aw-media-preview-toolbar-group aw-media-preview-toolbar-system">
              {onOpenExternal ? (
                <button
                  type="button"
                  aria-label={localOpenLabel}
                  title={localOpenLabel}
                  disabled={openingExternal}
                  onClick={onOpenExternal}
                >
                  <AwakenIcon concept="external-open" size={16} />
                  打开
                </button>
              ) : null}
              {onReveal ? (
                <button
                  type="button"
                  aria-label={revealLabel}
                  title={revealLabel}
                  onClick={onReveal}
                >
                  <AwakenIcon concept="folder-expanded" size={16} />
                  显示位置
                </button>
              ) : null}
            </div>
          </footer>
        ) : null}
      </div>
    </div>
  );
}
