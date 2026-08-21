import { useState } from "react";
import { Maximize2, Minus, Plus, RotateCcw } from "@awaken/design-system";
import type { ProjectFilePreviewUrlHandle } from "../../shared/runtime";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";

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
  const [fit, setFit] = useState(true);
  const [scale, setScale] = useState(1);

  function adjustScale(delta: number): void {
    setFit(false);
    setScale((current) => Math.min(8, Math.max(0.1, current + delta)));
  }

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
            {Math.round(scale * 100)}%
          </output>
          <button
            type="button"
            aria-label="缩小图片"
            onClick={() => adjustScale(-0.1)}
          >
            <Minus size={15} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-label="放大图片"
            onClick={() => adjustScale(0.1)}
          >
            <Plus size={15} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-pressed={fit}
            aria-label="适应窗口"
            title="适应窗口"
            onClick={() => {
              setFit(true);
              setScale(1);
            }}
          >
            <Maximize2 size={15} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-label="原始尺寸"
            title="原始尺寸"
            onClick={() => {
              setFit(false);
              setScale(1);
            }}
          >
            1:1
          </button>
          <button
            type="button"
            aria-label="重置图片"
            onClick={() => {
              setFit(true);
              setScale(1);
            }}
          >
            <RotateCcw size={15} aria-hidden="true" />
          </button>
        </>
      }
    >
      <div
        className={`project-file-image-preview__canvas${fit ? " is-fit" : ""}`}
      >
        <img
          src={preview.src}
          alt={preview.displayName}
          style={fit ? undefined : { transform: `scale(${scale})` }}
          onError={onLoadError}
        />
      </div>
    </PreviewFrame>
  );
}
