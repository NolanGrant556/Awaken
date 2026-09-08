import { useEffect, useRef, useState } from "react";
import { AwakenIcon } from "@awaken/design-system";
import {
  getDocument,
  GlobalWorkerOptions,
  RenderingCancelledException,
  type PDFDocumentProxy,
} from "pdfjs-dist";
import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
import type { ProjectFilePreviewUrlHandle } from "../../shared/runtime";
import { PreviewToolsMenu } from "./PreviewToolsMenu";
import PreviewFrame, { type PreviewFrameProps } from "./PreviewFrame";

GlobalWorkerOptions.workerSrc = workerUrl;

type PdfPreviewHandle = ProjectFilePreviewUrlHandle & {
  kind: "pdf";
};

export type PdfPreviewProps = {
  preview: PdfPreviewHandle;
  onLoadingChange?: (loading: boolean) => void;
  onLoadError: (message: string) => void;
  frameProps: Omit<
    PreviewFrameProps,
    "children" | "formatActions" | "formatLabel"
  >;
};

const MIN_SCALE = 0.25;
const MAX_SCALE = 4;
const SCALE_STEP = 0.25;
const PAGE_GUTTER = 48;

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) {
    return `PDF 加载失败：${error.message}`;
  }
  return "PDF 加载失败。";
}

export default function PdfPreview({
  preview,
  onLoadingChange,
  onLoadError,
  frameProps,
}: PdfPreviewProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const onLoadingChangeRef = useRef(onLoadingChange);
  const onLoadErrorRef = useRef(onLoadError);
  const [documentProxy, setDocumentProxy] = useState<PDFDocumentProxy>();
  const [pageNumber, setPageNumber] = useState(1);
  const [scale, setScale] = useState(1);
  const [fitWidth, setFitWidth] = useState(true);
  const [rotation, setRotation] = useState(0);
  const [viewportWidth, setViewportWidth] = useState(0);

  onLoadingChangeRef.current = onLoadingChange;
  onLoadErrorRef.current = onLoadError;

  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const updateWidth = () =>
      setViewportWidth(Math.floor(viewport.clientWidth));
    updateWidth();
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", updateWidth);
      return () => window.removeEventListener("resize", updateWidth);
    }
    const observer = new ResizeObserver(updateWidth);
    observer.observe(viewport);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    let active = true;
    setDocumentProxy(undefined);
    setPageNumber(1);
    setScale(1);
    setFitWidth(true);
    setRotation(0);
    onLoadingChangeRef.current?.(true);

    const loadingTask = getDocument({
      url: preview.src,
      rangeChunkSize: 64 * 1024,
      withCredentials: false,
    });
    void loadingTask.promise.then(
      (pdf) => {
        if (active) setDocumentProxy(pdf);
      },
      (error) => {
        if (!active) return;
        onLoadingChangeRef.current?.(false);
        onLoadErrorRef.current(errorMessage(error));
      },
    );

    return () => {
      active = false;
      void loadingTask.destroy();
    };
  }, [preview.previewId, preview.src]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !documentProxy) return;
    let active = true;
    let renderTask:
      | ReturnType<Awaited<ReturnType<PDFDocumentProxy["getPage"]>>["render"]>
      | undefined;
    let page: Awaited<ReturnType<PDFDocumentProxy["getPage"]>> | undefined;
    onLoadingChangeRef.current?.(true);

    void documentProxy
      .getPage(pageNumber)
      .then((loadedPage) => {
        if (!active) return;
        page = loadedPage;
        const unitViewport = loadedPage.getViewport({ scale: 1, rotation });
        const availableWidth = Math.max(240, viewportWidth - PAGE_GUTTER);
        const displayScale = fitWidth
          ? clamp(availableWidth / unitViewport.width, MIN_SCALE, MAX_SCALE)
          : scale;
        const outputScale = clamp(window.devicePixelRatio || 1, 1, 2);
        const displayViewport = loadedPage.getViewport({
          scale: displayScale,
          rotation,
        });
        const renderViewport = loadedPage.getViewport({
          scale: displayScale * outputScale,
          rotation,
        });

        canvas.width = Math.floor(renderViewport.width);
        canvas.height = Math.floor(renderViewport.height);
        canvas.style.width = `${Math.floor(displayViewport.width)}px`;
        canvas.style.height = `${Math.floor(displayViewport.height)}px`;
        renderTask = loadedPage.render({ canvas, viewport: renderViewport });
        return renderTask.promise;
      })
      .then(
        () => {
          if (active) onLoadingChangeRef.current?.(false);
        },
        (error) => {
          if (!active || error instanceof RenderingCancelledException) return;
          onLoadingChangeRef.current?.(false);
          onLoadErrorRef.current(errorMessage(error));
        },
      );

    return () => {
      active = false;
      renderTask?.cancel();
      page?.cleanup();
    };
  }, [documentProxy, fitWidth, pageNumber, rotation, scale, viewportWidth]);

  const pageCount = documentProxy?.numPages ?? 0;
  const changePage = (nextPage: number) => {
    if (!pageCount) return;
    setPageNumber(clamp(Math.round(nextPage), 1, pageCount));
  };
  const changeScale = (nextScale: number) => {
    setFitWidth(false);
    setScale(clamp(nextScale, MIN_SCALE, MAX_SCALE));
  };

  const zoomControls = (
    <div className="project-file-preview-heading__control-group">
      <button
        type="button"
        aria-label="缩小"
        title="缩小"
        onClick={() => changeScale((fitWidth ? 1 : scale) - SCALE_STEP)}
      >
        <AwakenIcon concept="collapse" size={16} />
      </button>
      <output aria-label="缩放比例">
        {fitWidth ? "适宽" : `${Math.round(scale * 100)}%`}
      </output>
      <button
        type="button"
        aria-label="放大"
        title="放大"
        onClick={() => changeScale((fitWidth ? 1 : scale) + SCALE_STEP)}
      >
        <AwakenIcon concept="add" size={16} />
      </button>
      <button
        type="button"
        aria-label="适应宽度"
        title="适应宽度"
        aria-pressed={fitWidth}
        onClick={() => setFitWidth(true)}
      >
        <AwakenIcon concept="fit-page" size={16} />
      </button>
      <button
        type="button"
        aria-label="顺时针旋转"
        title="顺时针旋转"
        onClick={() => setRotation((value) => (value + 90) % 360)}
      >
        <AwakenIcon concept="rotate-cw" size={16} />
      </button>
    </div>
  );

  return (
    <PreviewFrame
      {...frameProps}
      formatLabel="PDF"
      formatActions={
        <div
          className="project-file-pdf-preview__controls"
          aria-label="PDF 查看工具栏"
        >
          <div className="project-file-preview-heading__control-group">
            <button
              type="button"
              aria-label="上一页"
              title="上一页"
              disabled={!pageCount || pageNumber <= 1}
              onClick={() => changePage(pageNumber - 1)}
            >
              <AwakenIcon concept="back" size={16} />
            </button>
            <label className="project-file-pdf-preview__page">
              <input
                type="number"
                aria-label="页码"
                min={1}
                max={Math.max(1, pageCount)}
                value={pageNumber}
                disabled={!pageCount}
                onChange={(event) => changePage(Number(event.target.value))}
              />
              <span>/ {pageCount || "–"}</span>
            </label>
            <button
              type="button"
              aria-label="下一页"
              title="下一页"
              disabled={!pageCount || pageNumber >= pageCount}
              onClick={() => changePage(pageNumber + 1)}
            >
              <AwakenIcon concept="disclose" size={16} />
            </button>
          </div>
          <div className="project-file-pdf-preview__full-tools">
            {zoomControls}
          </div>
          <PreviewToolsMenu>{zoomControls}</PreviewToolsMenu>
        </div>
      }
    >
      <div ref={viewportRef} className="project-file-pdf-preview__viewport">
        <canvas
          ref={canvasRef}
          aria-label={`PDF 第 ${pageNumber} 页，共 ${pageCount} 页`}
        />
      </div>
    </PreviewFrame>
  );
}
