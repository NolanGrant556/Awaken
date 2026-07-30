import "./AwakenMediaPreviewOverlay.css";
import {
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  FolderOpen,
  Maximize2,
  Minus,
  Plus,
  RotateCcw,
  X,
} from "lucide-react";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type SyntheticEvent,
} from "react";

export type AwakenMediaPreviewItem = {
  id: string;
  kind: "image" | "video";
  title: string;
  src: string;
  alt?: string;
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
}: AwakenMediaPreviewOverlayProps) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const fullscreenRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<{
    pointerId: number;
    x: number;
    y: number;
    scrollLeft: number;
    scrollTop: number;
  } | undefined>(undefined);
  const [fit, setFit] = useState(true);
  const [scale, setScale] = useState(1);
  const [naturalSize, setNaturalSize] = useState<{ width: number; height: number }>();
  const [dragging, setDragging] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);

  useEffect(() => {
    setFit(true);
    setScale(1);
    setNaturalSize(undefined);
    setDragging(false);
    setLoadFailed(false);
    requestAnimationFrame(() => closeButtonRef.current?.focus());
  }, [item.id]);

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

  const setOriginalSize = () => {
    setFit(false);
    setScale(1);
  };

  const adjustScale = (delta: number) => {
    setFit(false);
    setScale((current) => Math.min(8, Math.max(0.1, Number((current + delta).toFixed(2)))));
  };

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

  const imageStageStyle =
    !fit && naturalSize
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
            ref={closeButtonRef}
            type="button"
            className="aw-media-preview-icon-button"
            aria-label="关闭媒体预览"
            onClick={onClose}
          >
            <X size={18} strokeWidth={1.9} aria-hidden="true" />
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
              <ChevronLeft size={22} strokeWidth={1.8} aria-hidden="true" />
            </button>
          ) : null}
          {onNext ? (
            <button
              type="button"
              className="aw-media-preview-nav aw-media-preview-nav-next"
              aria-label="查看下一个媒体"
              onClick={onNext}
            >
              <ChevronRight size={22} strokeWidth={1.8} aria-hidden="true" />
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
              {loadFailed ? (
                <span className="aw-media-preview-error">图片预览加载失败，可以尝试使用本地应用打开。</span>
              ) : (
                <span className="aw-media-preview-image-stage" style={imageStageStyle}>
                  <img
                    src={item.src}
                    alt={item.alt || item.title}
                    draggable={false}
                    onLoad={(event: SyntheticEvent<HTMLImageElement>) => {
                      const image = event.currentTarget;
                      setNaturalSize({ width: image.naturalWidth, height: image.naturalHeight });
                    }}
                    onError={() => setLoadFailed(true)}
                  />
                </span>
              )}
            </div>
          ) : (
            <div className="aw-media-preview-video-canvas">
              {loadFailed ? (
                <span className="aw-media-preview-error">视频预览加载失败，可以尝试使用本地应用打开。</span>
              ) : (
                <video
                  src={item.src}
                  controls
                  playsInline
                  preload="metadata"
                  aria-label={item.alt || item.title}
                  onError={() => setLoadFailed(true)}
                />
              )}
            </div>
          )}
        </div>

        <footer className="aw-media-preview-toolbar">
          <div className="aw-media-preview-toolbar-group aw-media-preview-zoom-controls">
            {item.kind === "image" ? (
              <>
                <button type="button" aria-label="缩小图片" onClick={() => adjustScale(-0.25)}>
                  <Minus size={15} aria-hidden="true" />
                </button>
                <span className="aw-media-preview-scale">
                  {fit ? "适应" : `${Math.round(scale * 100)}%`}
                </span>
                <button type="button" aria-label="放大图片" onClick={() => adjustScale(0.25)}>
                  <Plus size={15} aria-hidden="true" />
                </button>
                <button
                  type="button"
                  className="aw-media-preview-compact-button"
                  aria-label="适应窗口"
                  title="适应窗口"
                  onClick={() => {
                    setFit(true);
                    setScale(1);
                  }}
                >
                  <RotateCcw size={15} aria-hidden="true" />
                </button>
                <button
                  type="button"
                  className="aw-media-preview-compact-button"
                  aria-label="原始尺寸"
                  title="原始尺寸"
                  onClick={setOriginalSize}
                >
                  1:1
                </button>
              </>
            ) : null}
            <button
              type="button"
              className="aw-media-preview-compact-button"
              aria-label="全屏"
              title="全屏"
              onClick={() => {
                void fullscreenRef.current?.requestFullscreen?.();
              }}
            >
              <Maximize2 size={15} aria-hidden="true" />
            </button>
          </div>
          <div className="aw-media-preview-toolbar-group aw-media-preview-toolbar-system">
            {onOpenExternal ? (
              <button
                type="button"
                aria-label={localOpenLabel}
                title={localOpenLabel}
                disabled={openingExternal}
                onClick={onOpenExternal}
              >
                <ExternalLink size={15} aria-hidden="true" />
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
                <FolderOpen size={15} aria-hidden="true" />
                显示位置
              </button>
            ) : null}
          </div>
        </footer>
      </div>
    </div>
  );
}
