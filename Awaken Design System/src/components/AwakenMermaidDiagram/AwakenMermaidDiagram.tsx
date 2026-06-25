import {
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { AlertCircle, Check, Copy, Maximize2, RotateCcw, X, ZoomIn, ZoomOut } from "lucide-react";
import { AwakenTooltip } from "../AwakenTooltip";
import "./AwakenMermaidDiagram.css";

export type AwakenMermaidDiagramProps = {
  chart: string;
  className?: string;
};

type MermaidRenderState =
  | { status: "loading" }
  | { status: "ready"; svg: string }
  | { status: "error"; message: string };

type CopyState = "idle" | "copied" | "error";

type DiagramSize = {
  width: number;
  height: number;
};

type PanOffset = {
  x: number;
  y: number;
};

type PanState = {
  pointerId: number;
  startX: number;
  startY: number;
  panX: number;
  panY: number;
};

const MIN_EXPANDED_ZOOM = 0.5;
const MAX_EXPANDED_ZOOM = 8;
const DEFAULT_EXPANDED_ZOOM = 1;
const EXPANDED_ZOOM_STEP = 0.25;

let mermaidInitialized = false;

async function renderMermaid(id: string, chart: string) {
  const { default: mermaid } = await import("mermaid");

  if (!mermaidInitialized) {
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: "neutral",
    });
    mermaidInitialized = true;
  }

  return mermaid.render(id, chart);
}

function errorMessageFor(error: unknown): string {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return "Unable to render Mermaid diagram.";
}

async function writeTextToClipboard(text: string): Promise<void> {
  let clipboardError: unknown;

  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch (error) {
      clipboardError = error;
    }
  }

  if (typeof document === "undefined") {
    throw clipboardError || new Error("Clipboard is unavailable.");
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.top = "-1000px";
  textarea.style.left = "-1000px";
  document.body.appendChild(textarea);
  textarea.select();

  try {
    const copied = document.execCommand("copy");
    if (!copied) {
      throw clipboardError || new Error("Copy command was rejected.");
    }
  } finally {
    document.body.removeChild(textarea);
  }
}

function labelForCopyState(copyState: CopyState) {
  if (copyState === "copied") {
    return "Copied Mermaid source";
  }
  if (copyState === "error") {
    return "Copy failed";
  }
  return "Copy Mermaid source";
}

function iconForCopyState(copyState: CopyState) {
  if (copyState === "copied") {
    return <Check size={15} strokeWidth={2.4} />;
  }
  if (copyState === "error") {
    return <AlertCircle size={15} strokeWidth={2.2} />;
  }
  return <Copy size={15} strokeWidth={2} />;
}

function clampExpandedZoom(value: number): number {
  return Math.min(MAX_EXPANDED_ZOOM, Math.max(MIN_EXPANDED_ZOOM, value));
}

function formatZoomPercent(value: number): string {
  return `${Math.round(value * 100)}%`;
}

type MermaidDiagramToolbarProps = {
  copyState: CopyState;
  canExpand?: boolean;
  canCopy: boolean;
  expanded?: boolean;
  onCopy: () => void;
  onExpand?: () => void;
  onClose?: () => void;
};

function MermaidDiagramToolbar({
  copyState,
  canExpand = false,
  canCopy,
  expanded = false,
  onCopy,
  onExpand,
  onClose,
}: MermaidDiagramToolbarProps) {
  const copyLabel = labelForCopyState(copyState);

  return (
    <div className="assistant-mermaid-toolbar" aria-label="Mermaid diagram actions">
      {canExpand ? (
        <AwakenTooltip label="Expand diagram">
          <button
            type="button"
            className="assistant-mermaid-action"
            aria-label="Expand diagram"
            title="Expand diagram"
            onClick={onExpand}
          >
            <Maximize2 size={15} strokeWidth={2} />
          </button>
        </AwakenTooltip>
      ) : null}

      <AwakenTooltip label={copyLabel}>
        <button
          type="button"
          className={`assistant-mermaid-action assistant-mermaid-copy-action ${
            copyState === "copied"
              ? "is-copied"
              : copyState === "error"
                ? "is-error"
                : ""
          }`}
          aria-label={copyLabel}
          title={copyLabel}
          disabled={!canCopy}
          onClick={onCopy}
        >
          {iconForCopyState(copyState)}
        </button>
      </AwakenTooltip>

      {expanded ? (
        <AwakenTooltip label="Close expanded diagram">
          <button
            type="button"
            className="assistant-mermaid-action"
            aria-label="Close expanded diagram"
            title="Close expanded diagram"
            onClick={onClose}
          >
            <X size={16} strokeWidth={2} />
          </button>
        </AwakenTooltip>
      ) : null}
    </div>
  );
}

type MermaidDiagramZoomControlsProps = {
  zoom: number;
  canResetView: boolean;
  onZoomChange: (zoom: number) => void;
  onResetView: () => void;
};

function MermaidDiagramZoomControls({
  zoom,
  canResetView,
  onZoomChange,
  onResetView,
}: MermaidDiagramZoomControlsProps) {
  const zoomPercent = formatZoomPercent(zoom);
  const canZoomOut = zoom > MIN_EXPANDED_ZOOM;
  const canZoomIn = zoom < MAX_EXPANDED_ZOOM;

  return (
    <div className="assistant-mermaid-zoom-controls" aria-label="Mermaid diagram zoom">
      <AwakenTooltip label="Zoom out">
        <button
          type="button"
          className="assistant-mermaid-action"
          aria-label="Zoom out"
          title="Zoom out"
          disabled={!canZoomOut}
          onClick={() => onZoomChange(clampExpandedZoom(zoom - EXPANDED_ZOOM_STEP))}
        >
          <ZoomOut size={15} strokeWidth={2} />
        </button>
      </AwakenTooltip>

      <input
        className="assistant-mermaid-zoom-range"
        type="range"
        min={MIN_EXPANDED_ZOOM}
        max={MAX_EXPANDED_ZOOM}
        step={EXPANDED_ZOOM_STEP}
        value={zoom}
        aria-label="Diagram zoom"
        aria-valuetext={zoomPercent}
        onChange={(event) => onZoomChange(clampExpandedZoom(Number(event.currentTarget.value)))}
      />
      <span className="assistant-mermaid-zoom-value" aria-live="polite">
        {zoomPercent}
      </span>

      <AwakenTooltip label="Zoom in">
        <button
          type="button"
          className="assistant-mermaid-action"
          aria-label="Zoom in"
          title="Zoom in"
          disabled={!canZoomIn}
          onClick={() => onZoomChange(clampExpandedZoom(zoom + EXPANDED_ZOOM_STEP))}
        >
          <ZoomIn size={15} strokeWidth={2} />
        </button>
      </AwakenTooltip>

      <AwakenTooltip label="Reset view">
        <button
          type="button"
          className="assistant-mermaid-action"
          aria-label="Reset view"
          title="Reset view"
          disabled={!canResetView}
          onClick={onResetView}
        >
          <RotateCcw size={14} strokeWidth={2} />
        </button>
      </AwakenTooltip>
    </div>
  );
}

export function AwakenMermaidDiagram({
  chart,
  className,
}: AwakenMermaidDiagramProps) {
  const reactId = useId();
  const diagramId = `awaken-mermaid-${reactId.replace(/[^a-zA-Z0-9_-]/g, "")}`;
  const normalizedChart = chart.trim();
  const [renderState, setRenderState] = useState<MermaidRenderState>({ status: "loading" });
  const [copyState, setCopyState] = useState<CopyState>("idle");
  const [isExpanded, setIsExpanded] = useState(false);
  const [expandedZoom, setExpandedZoom] = useState(DEFAULT_EXPANDED_ZOOM);
  const [expandedDiagramSize, setExpandedDiagramSize] = useState<DiagramSize | null>(null);
  const [panOffset, setPanOffset] = useState<PanOffset>({ x: 0, y: 0 });
  const [isPanning, setIsPanning] = useState(false);
  const copyResetTimerRef = useRef<number | undefined>(undefined);
  const expandedDiagramRef = useRef<HTMLDivElement | null>(null);
  const panStateRef = useRef<PanState | null>(null);

  useEffect(() => {
    let cancelled = false;

    if (!normalizedChart) {
      setRenderState({ status: "error", message: "Mermaid diagram is empty." });
      return () => {
        cancelled = true;
      };
    }

    setRenderState({ status: "loading" });
    renderMermaid(diagramId, normalizedChart)
      .then(({ svg }) => {
        if (!cancelled) {
          setRenderState({ status: "ready", svg });
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setRenderState({ status: "error", message: errorMessageFor(error) });
        }
      });

    return () => {
      cancelled = true;
    };
  }, [diagramId, normalizedChart]);

  useEffect(() => {
    return () => {
      if (copyResetTimerRef.current !== undefined) {
        window.clearTimeout(copyResetTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    setCopyState("idle");
    setIsExpanded(false);
    setExpandedZoom(DEFAULT_EXPANDED_ZOOM);
    setExpandedDiagramSize(null);
    setPanOffset({ x: 0, y: 0 });
    setIsPanning(false);
    panStateRef.current = null;
  }, [normalizedChart]);

  useEffect(() => {
    if (!isExpanded) {
      panStateRef.current = null;
      setIsPanning(false);
      return undefined;
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setIsExpanded(false);
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isExpanded]);

  async function handleCopySource() {
    try {
      await writeTextToClipboard(normalizedChart);
      setCopyState("copied");
    } catch {
      setCopyState("error");
    }

    if (copyResetTimerRef.current !== undefined) {
      window.clearTimeout(copyResetTimerRef.current);
    }

    copyResetTimerRef.current = window.setTimeout(() => {
      setCopyState("idle");
    }, 1200);
  }

  function handleExpandDiagram() {
    setExpandedZoom(DEFAULT_EXPANDED_ZOOM);
    setExpandedDiagramSize(null);
    setPanOffset({ x: 0, y: 0 });
    setIsPanning(false);
    panStateRef.current = null;
    setIsExpanded(true);
  }

  function handleResetView() {
    setExpandedZoom(DEFAULT_EXPANDED_ZOOM);
    setPanOffset({ x: 0, y: 0 });
  }

  function handlePanStart(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button !== 0) {
      return;
    }

    panStateRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      panX: panOffset.x,
      panY: panOffset.y,
    };
    event.currentTarget.setPointerCapture(event.pointerId);
    setIsPanning(true);
    event.preventDefault();
  }

  function handlePanMove(event: ReactPointerEvent<HTMLDivElement>) {
    const panState = panStateRef.current;

    if (!panState || panState.pointerId !== event.pointerId) {
      return;
    }

    setPanOffset({
      x: panState.panX + event.clientX - panState.startX,
      y: panState.panY + event.clientY - panState.startY,
    });
    event.preventDefault();
  }

  function handlePanEnd(event: ReactPointerEvent<HTMLDivElement>) {
    const panState = panStateRef.current;

    if (!panState || panState.pointerId !== event.pointerId) {
      return;
    }

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }

    panStateRef.current = null;
    setIsPanning(false);
  }

  useEffect(() => {
    if (!isExpanded || renderState.status !== "ready") {
      return undefined;
    }

    const animationFrameId = window.requestAnimationFrame(() => {
      const svgElement = expandedDiagramRef.current?.querySelector("svg");

      if (!svgElement) {
        return;
      }

      const rect = svgElement.getBoundingClientRect();
      setExpandedDiagramSize({
        width: rect.width,
        height: rect.height,
      });
    });

    return () => {
      window.cancelAnimationFrame(animationFrameId);
    };
  }, [isExpanded, renderState]);

  const rootClassName = className
    ? `assistant-mermaid-diagram ${className}`
    : "assistant-mermaid-diagram";
  const canCopy = normalizedChart.length > 0;
  const canResetView =
    expandedZoom !== DEFAULT_EXPANDED_ZOOM || panOffset.x !== 0 || panOffset.y !== 0;
  const expandedFrameStyle = expandedDiagramSize
    ? ({
        "--assistant-mermaid-pan-x": `${panOffset.x}px`,
        "--assistant-mermaid-pan-y": `${panOffset.y}px`,
        width: `${expandedDiagramSize.width * expandedZoom}px`,
        height: `${expandedDiagramSize.height * expandedZoom}px`,
      } as CSSProperties)
    : ({
        "--assistant-mermaid-pan-x": `${panOffset.x}px`,
        "--assistant-mermaid-pan-y": `${panOffset.y}px`,
      } as CSSProperties);

  if (renderState.status === "ready") {
    return (
      <figure className={rootClassName} aria-label="Mermaid diagram">
        <MermaidDiagramToolbar
          copyState={copyState}
          canExpand
          canCopy={canCopy}
          onCopy={() => void handleCopySource()}
          onExpand={handleExpandDiagram}
        />
        <div
          className="assistant-mermaid-diagram-svg"
          dangerouslySetInnerHTML={{ __html: renderState.svg }}
        />
        {isExpanded ? (
          <div
            className="assistant-mermaid-expanded-backdrop"
            role="presentation"
            onClick={() => setIsExpanded(false)}
          >
            <div
              className="assistant-mermaid-expanded-panel"
              role="dialog"
              aria-modal="true"
              aria-label="Expanded Mermaid diagram"
              onClick={(event) => {
                event.stopPropagation();
              }}
            >
              <div className="assistant-mermaid-expanded-head">
                <MermaidDiagramZoomControls
                  zoom={expandedZoom}
                  canResetView={canResetView}
                  onZoomChange={setExpandedZoom}
                  onResetView={handleResetView}
                />
                <MermaidDiagramToolbar
                  copyState={copyState}
                  canCopy={canCopy}
                  expanded
                  onCopy={() => void handleCopySource()}
                  onClose={() => setIsExpanded(false)}
                />
              </div>
              <div
                className={`assistant-mermaid-expanded-scroll${isPanning ? " is-panning" : ""}`}
                onPointerDown={handlePanStart}
                onPointerMove={handlePanMove}
                onPointerUp={handlePanEnd}
                onPointerCancel={handlePanEnd}
              >
                <div
                  className="assistant-mermaid-expanded-zoom-frame"
                  style={expandedFrameStyle}
                >
                  <div
                    ref={expandedDiagramRef}
                    className="assistant-mermaid-diagram-svg assistant-mermaid-expanded-svg"
                    dangerouslySetInnerHTML={{ __html: renderState.svg }}
                  />
                </div>
              </div>
            </div>
          </div>
        ) : null}
      </figure>
    );
  }

  if (renderState.status === "error") {
    return (
      <figure className={`${rootClassName} assistant-mermaid-diagram-error`}>
        <div className="assistant-mermaid-error-head">
          <figcaption>Unable to render Mermaid diagram.</figcaption>
          <MermaidDiagramToolbar
            copyState={copyState}
            canCopy={canCopy}
            onCopy={() => void handleCopySource()}
          />
        </div>
        <p>{renderState.message}</p>
        <pre className="assistant-message-code-block">
          <code className="assistant-message-code language-mermaid">{chart}</code>
        </pre>
      </figure>
    );
  }

  return (
    <figure className={`${rootClassName} assistant-mermaid-diagram-loading`}>
      Rendering diagram...
    </figure>
  );
}
