import { useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { AwakenIcon } from "@awaken/design-system";
import type { ProjectFilePreviewUrlHandle } from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";

const IMAGE_PREVIEW_GUTTER = 24;
const IMAGE_ZOOM_FACTOR = 1.1;
const MIN_IMAGE_SCALE = 0.000001;
const MAX_IMAGE_SCALE = 8;

type ImageDimensions = {
  width: number;
  height: number;
};

type LoadedImageDimensions = ImageDimensions & {
  previewId: string;
};

type ImageZoomMode = "fit" | "manual";

type ImageViewportFocus = {
  x: number;
  y: number;
};

export function imageFitScale({
  imageWidth,
  imageHeight,
  viewportWidth,
  viewportHeight,
}: {
  imageWidth: number;
  imageHeight: number;
  viewportWidth: number;
  viewportHeight: number;
}): number {
  if (
    imageWidth <= 0 ||
    imageHeight <= 0 ||
    viewportWidth <= 0 ||
    viewportHeight <= 0
  ) {
    return 1;
  }
  return Math.min(1, viewportWidth / imageWidth, viewportHeight / imageHeight);
}

export function nextImageZoomScale(
  currentScale: number,
  direction: "in" | "out",
): number {
  const nextScale =
    direction === "in"
      ? currentScale * IMAGE_ZOOM_FACTOR
      : currentScale / IMAGE_ZOOM_FACTOR;
  return Math.min(MAX_IMAGE_SCALE, Math.max(MIN_IMAGE_SCALE, nextScale));
}

export function imagePointAtViewportCenter({
  scrollOffset,
  viewportSize,
  imageOffset,
  scale,
}: {
  scrollOffset: number;
  viewportSize: number;
  imageOffset: number;
  scale: number;
}): number {
  if (scale <= 0) return 0;
  return (scrollOffset + viewportSize / 2 - imageOffset) / scale;
}

export function scrollOffsetForCenteredImagePoint({
  imagePoint,
  viewportSize,
  imageOffset,
  scale,
}: {
  imagePoint: number;
  viewportSize: number;
  imageOffset: number;
  scale: number;
}): number {
  return imageOffset + imagePoint * scale - viewportSize / 2;
}

function formatImageScale(scale: number): string {
  const percentage = scale * 100;
  if (percentage > 0 && percentage < 1) return "<1%";
  return `${Math.round(percentage)}%`;
}

function cssPixels(value: number): string {
  return `${Math.round(value * 1000) / 1000}px`;
}

export type ImagePreviewProps = {
  preview: ProjectFilePreviewUrlHandle;
  onLoadError: () => void;
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
};

export default function ImagePreview({
  preview,
  onLoadError,
  frameProps,
}: ImagePreviewProps) {
  const canvasRef = useRef<HTMLDivElement>(null);
  const imageRef = useRef<HTMLImageElement>(null);
  const pendingFocusRef = useRef<ImageViewportFocus | undefined>(undefined);
  const [zoomMode, setZoomMode] = useState<ImageZoomMode>("fit");
  const [manualScale, setManualScale] = useState(1);
  const [viewportSize, setViewportSize] = useState<ImageDimensions>({
    width: 0,
    height: 0,
  });
  const [loadedImage, setLoadedImage] = useState<LoadedImageDimensions>();

  const imageSize =
    loadedImage?.previewId === preview.previewId ? loadedImage : undefined;
  const hasMeasuredImage =
    Boolean(imageSize) && viewportSize.width > 0 && viewportSize.height > 0;
  const fitScale = imageSize
    ? imageFitScale({
        imageWidth: imageSize.width,
        imageHeight: imageSize.height,
        viewportWidth: viewportSize.width,
        viewportHeight: viewportSize.height,
      })
    : 1;
  const effectiveZoomMode = hasMeasuredImage ? zoomMode : "fit";
  const effectiveScale = effectiveZoomMode === "fit" ? fitScale : manualScale;
  const renderedWidth = imageSize && hasMeasuredImage
    ? imageSize.width * effectiveScale
    : undefined;
  const renderedHeight = imageSize && hasMeasuredImage
    ? imageSize.height * effectiveScale
    : undefined;
  const canZoomOut = hasMeasuredImage && effectiveScale > MIN_IMAGE_SCALE;
  const canZoomIn = hasMeasuredImage && effectiveScale < MAX_IMAGE_SCALE;
  const shouldContractToFit =
    hasMeasuredImage && effectiveScale > fitScale * (1 + 1e-9);
  const fitActionTitle = shouldContractToFit ? "恢复适应窗口" : "适应窗口";

  useLayoutEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return undefined;

    const updateViewportSize = (): void => {
      const nextSize = {
        width: Math.max(0, canvas.clientWidth - IMAGE_PREVIEW_GUTTER * 2),
        height: Math.max(0, canvas.clientHeight - IMAGE_PREVIEW_GUTTER * 2),
      };
      setViewportSize((current) =>
        current.width === nextSize.width && current.height === nextSize.height
          ? current
          : nextSize,
      );
    };

    updateViewportSize();
    const observer = new ResizeObserver(updateViewportSize);
    observer.observe(canvas);
    return () => observer.disconnect();
  }, []);

  useLayoutEffect(() => {
    pendingFocusRef.current = undefined;
    setZoomMode("fit");
    setManualScale(1);
    setLoadedImage(undefined);
    const canvas = canvasRef.current;
    if (canvas) {
      canvas.scrollLeft = 0;
      canvas.scrollTop = 0;
    }
  }, [preview.previewId]);

  useLayoutEffect(() => {
    const canvas = canvasRef.current;
    const image = imageRef.current;
    if (!canvas || !image || !imageSize) return;

    if (effectiveZoomMode === "fit") {
      pendingFocusRef.current = undefined;
      canvas.scrollLeft = 0;
      canvas.scrollTop = 0;
      return;
    }

    const pendingFocus = pendingFocusRef.current;
    if (!pendingFocus) return;

    const canvasRect = canvas.getBoundingClientRect();
    const imageRect = image.getBoundingClientRect();
    const imageLeft = imageRect.left - canvasRect.left + canvas.scrollLeft;
    const imageTop = imageRect.top - canvasRect.top + canvas.scrollTop;
    canvas.scrollLeft = scrollOffsetForCenteredImagePoint({
      imagePoint: pendingFocus.x,
      viewportSize: canvas.clientWidth,
      imageOffset: imageLeft,
      scale: effectiveScale,
    });
    canvas.scrollTop = scrollOffsetForCenteredImagePoint({
      imagePoint: pendingFocus.y,
      viewportSize: canvas.clientHeight,
      imageOffset: imageTop,
      scale: effectiveScale,
    });
    pendingFocusRef.current = undefined;
  }, [effectiveScale, effectiveZoomMode, imageSize]);

  function captureViewportFocus(): ImageViewportFocus | undefined {
    const canvas = canvasRef.current;
    const image = imageRef.current;
    if (!canvas || !image || !imageSize) return undefined;

    const canvasRect = canvas.getBoundingClientRect();
    const imageRect = image.getBoundingClientRect();
    const imageLeft = imageRect.left - canvasRect.left + canvas.scrollLeft;
    const imageTop = imageRect.top - canvasRect.top + canvas.scrollTop;
    return {
      x: imagePointAtViewportCenter({
        scrollOffset: canvas.scrollLeft,
        viewportSize: canvas.clientWidth,
        imageOffset: imageLeft,
        scale: effectiveScale,
      }),
      y: imagePointAtViewportCenter({
        scrollOffset: canvas.scrollTop,
        viewportSize: canvas.clientHeight,
        imageOffset: imageTop,
        scale: effectiveScale,
      }),
    };
  }

  function applyManualScale(nextScale: number): void {
    if (
      !hasMeasuredImage ||
      (effectiveZoomMode === "manual" && nextScale === effectiveScale)
    ) {
      return;
    }
    pendingFocusRef.current = captureViewportFocus();
    setZoomMode("manual");
    setManualScale(nextScale);
  }

  function useFitScale(): void {
    pendingFocusRef.current = undefined;
    setZoomMode("fit");
    const canvas = canvasRef.current;
    if (canvas) {
      canvas.scrollLeft = 0;
      canvas.scrollTop = 0;
    }
  }

  const stageStyle =
    renderedWidth !== undefined && renderedHeight !== undefined
      ? ({
          width: `max(100%, ${cssPixels(renderedWidth)})`,
          height: `max(100%, ${cssPixels(renderedHeight)})`,
        } satisfies CSSProperties)
      : undefined;
  const imageStyle =
    renderedWidth !== undefined && renderedHeight !== undefined
      ? ({
          width: cssPixels(renderedWidth),
          height: cssPixels(renderedHeight),
        } satisfies CSSProperties)
      : ({ visibility: "hidden" } satisfies CSSProperties);

  return (
    <PreviewFrame
      {...frameProps}
      formatLabel="图片"
      formatActions={
        <>
          <output
            className="project-file-preview-heading__value"
            aria-label="图片缩放比例"
          >
            {hasMeasuredImage ? formatImageScale(effectiveScale) : "—"}
          </output>
          <button
            type="button"
            aria-label="缩小图片"
            disabled={!canZoomOut}
            onClick={() =>
              applyManualScale(nextImageZoomScale(effectiveScale, "out"))
            }
          >
            <AwakenIcon concept="collapse" size={16} />
          </button>
          <button
            type="button"
            aria-label="放大图片"
            disabled={!canZoomIn}
            onClick={() =>
              applyManualScale(nextImageZoomScale(effectiveScale, "in"))
            }
          >
            <AwakenIcon concept="add" size={16} />
          </button>
          <button
            type="button"
            aria-pressed={effectiveZoomMode === "fit"}
            aria-label="适应窗口"
            title={fitActionTitle}
            disabled={!hasMeasuredImage}
            onClick={useFitScale}
          >
            <AwakenIcon
              concept={shouldContractToFit ? "exit-full" : "expand-full"}
              size={16}
            />
          </button>
          <button
            type="button"
            aria-label="原始尺寸"
            title="原始尺寸"
            disabled={!hasMeasuredImage}
            onClick={() => applyManualScale(1)}
          >
            1:1
          </button>
          <button
            type="button"
            aria-label="重置图片"
            disabled={!hasMeasuredImage}
            onClick={useFitScale}
          >
            <AwakenIcon concept="rotate-ccw" size={16} />
          </button>
        </>
      }
    >
      <div ref={canvasRef} className="project-file-image-preview__canvas">
        <div className="project-file-image-preview__stage" style={stageStyle}>
          <img
            ref={imageRef}
            src={preview.src}
            alt={preview.displayName}
            draggable={false}
            style={imageStyle}
            onLoad={(event) => {
              const image = event.currentTarget;
              if (image.naturalWidth <= 0 || image.naturalHeight <= 0) return;
              setLoadedImage({
                previewId: preview.previewId,
                width: image.naturalWidth,
                height: image.naturalHeight,
              });
            }}
            onError={onLoadError}
          />
        </div>
      </div>
    </PreviewFrame>
  );
}
