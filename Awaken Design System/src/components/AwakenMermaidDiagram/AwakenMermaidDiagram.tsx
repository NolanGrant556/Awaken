import {
  type CSSProperties,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { AwakenIcon } from "../AwakenIcon";
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

// SVG 渲染结果缓存：会话切换/重挂载时同一份图直接复用，
// 避免重复的动态 import + 异步渲染带来的 loading 闪烁与高度抖动。
// 缓存内容只是模板；写入 DOM 前必须按挂载实例重新作用域化内部 ID。
const MERMAID_SVG_CACHE_LIMIT = 50;
const mermaidSvgCache = new Map<string, string>();

const SVG_ID_REFERENCE_LIST_ATTRIBUTES = new Set([
  "aria-activedescendant",
  "aria-controls",
  "aria-describedby",
  "aria-details",
  "aria-labelledby",
  "aria-owns",
  "for",
  "headers",
]);

const SVG_LOCAL_HREF_ATTRIBUTES = new Set(["href", "xlink:href"]);

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function replaceUrlFragmentReferences(
  value: string,
  scopedIds: ReadonlyMap<string, string>,
): string {
  return value.replace(
    /url\(\s*["']?[^)]*?#([^)"'\s]+)["']?\s*\)/g,
    (reference, referencedId: string) => {
      const scopedId = scopedIds.get(referencedId);
      return scopedId ? reference.replace(`#${referencedId}`, `#${scopedId}`) : reference;
    },
  );
}

function replaceLocalHrefReference(
  value: string,
  scopedIds: ReadonlyMap<string, string>,
): string {
  const match = value.match(/^(\s*)#([^\s]+)(\s*)$/);
  if (!match) {
    return value;
  }

  const scopedId = scopedIds.get(match[2]);
  return scopedId ? `${match[1]}#${scopedId}${match[3]}` : value;
}

function replaceIdReferenceList(
  value: string,
  scopedIds: ReadonlyMap<string, string>,
): string {
  return value
    .split(/(\s+)/)
    .map((token) => scopedIds.get(token) ?? token)
    .join("");
}

function replaceTimingReferences(
  value: string,
  scopedIds: ReadonlyMap<string, string>,
): string {
  return value.replace(
    /(^|[;\s])([^;\s]+?)(?=\.)/g,
    (reference, prefix: string, referencedId: string) => {
      const scopedId = scopedIds.get(referencedId);
      return scopedId ? `${prefix}${scopedId}` : reference;
    },
  );
}

function scopeMermaidSvgIds(svg: string, scope: string): string {
  if (typeof DOMParser === "undefined" || typeof XMLSerializer === "undefined") {
    return svg;
  }

  const document = new DOMParser().parseFromString(svg, "image/svg+xml");
  if (document.querySelector("parsererror")) {
    return svg;
  }

  const root = document.documentElement;
  const elements = [root, ...Array.from(root.querySelectorAll("*"))];
  const scopedIds = new Map<string, string>();
  const safeScope = scope.replace(/[^a-zA-Z0-9_-]/g, "-");
  const normalizedScope = /^[a-zA-Z_]/.test(safeScope) ? safeScope : `awaken-${safeScope}`;

  for (const element of elements) {
    const currentId = element.getAttribute("id");
    if (!currentId || scopedIds.has(currentId)) {
      continue;
    }
    scopedIds.set(currentId, `${normalizedScope}-${scopedIds.size}`);
  }

  if (scopedIds.size === 0) {
    return svg;
  }

  const originalRootId = root.getAttribute("id");
  for (const element of elements) {
    const currentId = element.getAttribute("id");
    if (currentId) {
      const scopedId = scopedIds.get(currentId);
      if (scopedId) {
        element.setAttribute("id", scopedId);
      }
    }

    for (const attribute of Array.from(element.attributes)) {
      if (attribute.name === "id") {
        continue;
      }

      let nextValue = replaceUrlFragmentReferences(attribute.value, scopedIds);
      if (SVG_LOCAL_HREF_ATTRIBUTES.has(attribute.name)) {
        nextValue = replaceLocalHrefReference(nextValue, scopedIds);
      } else if (SVG_ID_REFERENCE_LIST_ATTRIBUTES.has(attribute.name)) {
        nextValue = replaceIdReferenceList(nextValue, scopedIds);
      } else if (attribute.name === "begin" || attribute.name === "end") {
        nextValue = replaceTimingReferences(nextValue, scopedIds);
      }

      if (nextValue !== attribute.value) {
        element.setAttributeNS(attribute.namespaceURI, attribute.name, nextValue);
      }
    }
  }

  if (originalRootId) {
    const scopedRootId = scopedIds.get(originalRootId);
    if (scopedRootId) {
      const rootSelector = new RegExp(
        `#${escapeRegExp(originalRootId)}(?=[^a-zA-Z0-9_-]|$)`,
        "g",
      );
      for (const styleElement of Array.from(root.querySelectorAll("style"))) {
        const currentCss = styleElement.textContent ?? "";
        styleElement.textContent = replaceUrlFragmentReferences(
          currentCss.replace(rootSelector, `#${scopedRootId}`),
          scopedIds,
        );
      }
    }
  }

  return new XMLSerializer().serializeToString(root);
}

function readCachedMermaidSvg(chart: string): string | null {
  return mermaidSvgCache.get(chart) ?? null;
}

function writeCachedMermaidSvg(chart: string, svg: string): void {
  mermaidSvgCache.delete(chart);
  mermaidSvgCache.set(chart, svg);
  while (mermaidSvgCache.size > MERMAID_SVG_CACHE_LIMIT) {
    const oldest = mermaidSvgCache.keys().next().value;
    if (oldest === undefined) {
      break;
    }
    mermaidSvgCache.delete(oldest);
  }
}

async function renderMermaid(id: string, chart: string) {
  const cached = readCachedMermaidSvg(chart);
  if (cached !== null) {
    return { svg: cached };
  }

  const { default: mermaid } = await import("mermaid");

  if (!mermaidInitialized) {
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: "neutral",
    });
    mermaidInitialized = true;
  }

  const result = await mermaid.render(id, chart);
  writeCachedMermaidSvg(chart, result.svg);
  return result;
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
    return <AwakenIcon concept="step-done" size={16} />;
  }
  if (copyState === "error") {
    return <AwakenIcon concept="error" size={16} />;
  }
  return <AwakenIcon concept="copy" size={16} />;
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
            <AwakenIcon concept="expand-full" size={16} />
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
        <button
          type="button"
          className="assistant-mermaid-action"
          aria-label="Close expanded diagram"
          onClick={onClose}
        >
          <AwakenIcon concept="close" size={16} />
        </button>
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
          <AwakenIcon concept="zoom-out" size={16} />
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
          <AwakenIcon concept="zoom-in" size={16} />
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
          <AwakenIcon concept="rotate-ccw" size={14} />
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
  const [renderState, setRenderState] = useState<MermaidRenderState>(() => {
    const cached = readCachedMermaidSvg(normalizedChart);
    return cached !== null ? { status: "ready", svg: cached } : { status: "loading" };
  });
  const [copyState, setCopyState] = useState<CopyState>("idle");
  const [isExpanded, setIsExpanded] = useState(false);
  const [expandedZoom, setExpandedZoom] = useState(DEFAULT_EXPANDED_ZOOM);
  const [expandedDiagramSize, setExpandedDiagramSize] = useState<DiagramSize | null>(null);
  const [panOffset, setPanOffset] = useState<PanOffset>({ x: 0, y: 0 });
  const [isPanning, setIsPanning] = useState(false);
  const copyResetTimerRef = useRef<number | undefined>(undefined);
  const expandedDiagramRef = useRef<HTMLDivElement | null>(null);
  const expandedScrollRef = useRef<HTMLDivElement | null>(null);
  const panStateRef = useRef<PanState | null>(null);
  const panOffsetRef = useRef<PanOffset>({ x: 0, y: 0 });
  panOffsetRef.current = panOffset;

  useEffect(() => {
    let cancelled = false;

    if (!normalizedChart) {
      setRenderState({ status: "error", message: "Mermaid diagram is empty." });
      return () => {
        cancelled = true;
      };
    }

    setRenderState(() => {
      const cached = readCachedMermaidSvg(normalizedChart);
      return cached !== null ? { status: "ready", svg: cached } : { status: "loading" };
    });
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

  function centerExpandedScroll() {
    const element = expandedScrollRef.current;
    if (!element) {
      return;
    }
    element.scrollLeft = (element.scrollWidth - element.clientWidth) / 2;
    element.scrollTop = (element.scrollHeight - element.clientHeight) / 2;
  }

  function handleResetView() {
    setExpandedZoom(DEFAULT_EXPANDED_ZOOM);
    setPanOffset({ x: 0, y: 0 });
    window.requestAnimationFrame(() => {
      centerExpandedScroll();
    });
  }

  // 手势起点监听必须用原生事件：实测发现此渲染链路中 React 委托的
  // onPointerDown 对真实鼠标输入不派发（原生事件正常冒泡到 #root，
  // 直接调用 prop 与 JS dispatchEvent 均正常），合成事件层在此断裂，
  // 因此 pointerdown 绕过 React 合成事件直接挂在 DOM 上。
  useEffect(() => {
    if (!isExpanded || renderState.status !== "ready") {
      return undefined;
    }

    const element = expandedScrollRef.current;
    if (!element) {
      return undefined;
    }
    const scrollElement: HTMLDivElement = element;

    function handleNativePointerDown(event: PointerEvent) {
      if (event.button !== 0) {
        return;
      }

      panStateRef.current = {
        pointerId: event.pointerId,
        startX: event.clientX,
        startY: event.clientY,
        panX: panOffsetRef.current.x,
        panY: panOffsetRef.current.y,
      };
      try {
        scrollElement.setPointerCapture(event.pointerId);
      } catch {
        // capture 不可用时 window 级 move/up 监听仍保证手势完整
      }
      setIsPanning(true);
      event.preventDefault();
    }

    element.addEventListener("pointerdown", handleNativePointerDown);
    return () => {
      element.removeEventListener("pointerdown", handleNativePointerDown);
    };
  }, [isExpanded, renderState.status]);

  useEffect(() => {
    if (!isPanning) {
      return undefined;
    }

    function handleWindowPointerMove(event: PointerEvent) {
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

    function handleWindowPointerEnd(event: PointerEvent) {
      const panState = panStateRef.current;

      if (!panState || panState.pointerId !== event.pointerId) {
        return;
      }

      panStateRef.current = null;
      setIsPanning(false);
    }

    window.addEventListener("pointermove", handleWindowPointerMove);
    window.addEventListener("pointerup", handleWindowPointerEnd);
    window.addEventListener("pointercancel", handleWindowPointerEnd);
    return () => {
      window.removeEventListener("pointermove", handleWindowPointerMove);
      window.removeEventListener("pointerup", handleWindowPointerEnd);
      window.removeEventListener("pointercancel", handleWindowPointerEnd);
    };
  }, [isPanning]);

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
      centerExpandedScroll();
    });

    return () => {
      window.cancelAnimationFrame(animationFrameId);
    };
  }, [isExpanded, renderState]);

  // zoom 变化时保持视口中心点稳定（scroll 驱动 pan 后无需重置浏览位置）
  const previousExpandedZoomRef = useRef(expandedZoom);
  useEffect(() => {
    const element = expandedScrollRef.current;
    const previousZoom = previousExpandedZoomRef.current;
    previousExpandedZoomRef.current = expandedZoom;

    if (!element || !isExpanded || previousZoom === expandedZoom) {
      return;
    }

    const ratio = expandedZoom / previousZoom;
    element.scrollLeft = (element.scrollLeft + element.clientWidth / 2) * ratio - element.clientWidth / 2;
    element.scrollTop = (element.scrollTop + element.clientHeight / 2) * ratio - element.clientHeight / 2;
  }, [expandedZoom, isExpanded]);

  const rootClassName = className
    ? `assistant-mermaid-diagram ${className}`
    : "assistant-mermaid-diagram";
  const canCopy = normalizedChart.length > 0;
  const canResetView =
    expandedZoom !== DEFAULT_EXPANDED_ZOOM || panOffset.x !== 0 || panOffset.y !== 0;
  const scopedInlineSvg = useMemo(() => {
    if (renderState.status !== "ready") {
      return null;
    }
    return scopeMermaidSvgIds(renderState.svg, `${diagramId}-inline`);
  }, [diagramId, renderState]);
  const scopedExpandedSvg = useMemo(() => {
    if (!isExpanded || renderState.status !== "ready") {
      return null;
    }
    return scopeMermaidSvgIds(renderState.svg, `${diagramId}-expanded`);
  }, [diagramId, isExpanded, renderState]);
  const expandedFrameStyle: CSSProperties = {
    transform: `translate(${panOffset.x}px, ${panOffset.y}px)`,
    ...(expandedDiagramSize
      ? {
          width: `${expandedDiagramSize.width * expandedZoom}px`,
          height: `${expandedDiagramSize.height * expandedZoom}px`,
        }
      : {}),
  };

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
          dangerouslySetInnerHTML={{ __html: scopedInlineSvg ?? renderState.svg }}
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
                ref={expandedScrollRef}
                className={`assistant-mermaid-expanded-scroll${isPanning ? " is-panning" : ""}`}
              >
                <div
                  className="assistant-mermaid-expanded-zoom-frame"
                  style={expandedFrameStyle}
                >
                  <div
                    ref={expandedDiagramRef}
                    className="assistant-mermaid-diagram-svg assistant-mermaid-expanded-svg"
                    dangerouslySetInnerHTML={{ __html: scopedExpandedSvg ?? renderState.svg }}
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
        <div
          className="assistant-message-code-frame assistant-message-code-frame-labeled"
          data-language="mermaid"
        >
          <div className="assistant-message-code-language">mermaid</div>
          <pre className="assistant-message-code-block">
            <code className="assistant-message-code language-mermaid">{chart}</code>
          </pre>
        </div>
      </figure>
    );
  }

  return (
    <figure className={`${rootClassName} assistant-mermaid-diagram-loading`}>
      Rendering diagram...
    </figure>
  );
}
