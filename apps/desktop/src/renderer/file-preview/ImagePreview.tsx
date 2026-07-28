import { useState } from "react";
import { Maximize2, Minus, Plus, RotateCcw } from "lucide-react";
import type { ProjectFilePreviewUrlHandle } from "../../shared/runtime";

export type ImagePreviewProps = {
  preview: ProjectFilePreviewUrlHandle;
  onLoadError: () => void;
};

export default function ImagePreview({ preview, onLoadError }: ImagePreviewProps) {
  const [fit, setFit] = useState(true);
  const [scale, setScale] = useState(1);

  function adjustScale(delta: number): void {
    setFit(false);
    setScale((current) => Math.min(8, Math.max(0.1, current + delta)));
  }

  return (
    <div className="project-file-image-preview" data-preview-kind="image">
      <div className="project-file-preview-toolbar">
        <span>{Math.round(scale * 100)}%</span>
        <div className="project-file-preview-toolbar__actions">
          <button type="button" aria-label="缩小图片" onClick={() => adjustScale(-0.1)}>
            <Minus size={15} aria-hidden="true" />
          </button>
          <button type="button" aria-label="放大图片" onClick={() => adjustScale(0.1)}>
            <Plus size={15} aria-hidden="true" />
          </button>
          <button
            type="button"
            aria-pressed={fit}
            onClick={() => {
              setFit(true);
              setScale(1);
            }}
          >
            <Maximize2 size={15} aria-hidden="true" />
            适应窗口
          </button>
          <button
            type="button"
            onClick={() => {
              setFit(false);
              setScale(1);
            }}
          >
            原始尺寸
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
        </div>
      </div>
      <div className={`project-file-image-preview__canvas${fit ? " is-fit" : ""}`}>
        <img
          src={preview.src}
          alt={preview.displayName}
          style={fit ? undefined : { transform: `scale(${scale})` }}
          onError={onLoadError}
        />
      </div>
    </div>
  );
}
