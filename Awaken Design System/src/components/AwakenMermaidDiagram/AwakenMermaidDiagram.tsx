import { useEffect, useId, useRef, useState } from "react";
import { AlertCircle, Check, Copy, Maximize2, X } from "lucide-react";
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
  const copyResetTimerRef = useRef<number | undefined>(undefined);

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
  }, [normalizedChart]);

  useEffect(() => {
    if (!isExpanded) {
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

  const rootClassName = className
    ? `assistant-mermaid-diagram ${className}`
    : "assistant-mermaid-diagram";
  const canCopy = normalizedChart.length > 0;

  if (renderState.status === "ready") {
    return (
      <figure className={rootClassName} aria-label="Mermaid diagram">
        <MermaidDiagramToolbar
          copyState={copyState}
          canExpand
          canCopy={canCopy}
          onCopy={() => void handleCopySource()}
          onExpand={() => setIsExpanded(true)}
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
              <MermaidDiagramToolbar
                copyState={copyState}
                canCopy={canCopy}
                expanded
                onCopy={() => void handleCopySource()}
                onClose={() => setIsExpanded(false)}
              />
              <div className="assistant-mermaid-expanded-scroll">
                <div
                  className="assistant-mermaid-diagram-svg"
                  dangerouslySetInnerHTML={{ __html: renderState.svg }}
                />
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
