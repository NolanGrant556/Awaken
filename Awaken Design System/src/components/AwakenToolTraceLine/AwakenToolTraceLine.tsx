import "./AwakenToolTraceLine.css";
import {
  AlertCircle,
  Ban,
  BookOpenText,
  ChevronRight,
  CircleSlash,
  Copy,
  LoaderCircle,
  OctagonX,
  PlugZap,
  ShieldOff,
  TerminalSquare,
  Wrench,
} from "lucide-react";
import { useId, useState, type ReactNode } from "react";

export type ToolTraceStatus =
  | "active"
  | "done"
  | "error"
  | "denied"
  | "cancelled"
  | "interrupted"
  | "blocked"
  | "unsupported";

export type ToolTraceKind = "tool" | "mcp" | "skill" | "terminal";

export type ToolTraceDetailSection = {
  label: string;
  content: string;
  format: "text" | "code" | "json" | "diff";
  tone?: "default" | "muted" | "command" | "danger" | "success";
};

export type ToolTraceDetailCard = {
  title: string;
  copyText?: string;
  sections: ToolTraceDetailSection[];
};

export type AwakenToolTraceLineProps = {
  title: string;
  /** Compact one-line context shown beside the title. */
  detail?: string;
  /** Bounded command or tool result rendered in the nested output card. */
  output?: string;
  outputLabel?: string;
  outputTruncated?: boolean;
  detailCard?: ToolTraceDetailCard;
  status?: ToolTraceStatus;
  kind?: ToolTraceKind;
  icon?: ReactNode;
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  onCopyDetail?: (copyText: string) => void;
  /** Disable the mount animation when a consumer reveals already-finished history. */
  animateEntry?: boolean;
  onSelect?: () => void;
};

function statusIcon(status: ToolTraceStatus, kind: ToolTraceKind) {
  switch (status) {
    case "active":
      return <LoaderCircle size={14} strokeWidth={2.1} aria-hidden="true" />;
    case "error":
      return <AlertCircle size={14} strokeWidth={2.1} aria-hidden="true" />;
    case "denied":
      return <Ban size={14} strokeWidth={2.1} aria-hidden="true" />;
    case "cancelled":
      return <CircleSlash size={14} strokeWidth={2.1} aria-hidden="true" />;
    case "interrupted":
      return <OctagonX size={14} strokeWidth={2.1} aria-hidden="true" />;
    case "blocked":
      return <ShieldOff size={14} strokeWidth={2.1} aria-hidden="true" />;
    case "unsupported":
      return <AlertCircle size={14} strokeWidth={2.1} aria-hidden="true" />;
    default:
      if (kind === "mcp") return <PlugZap size={14} strokeWidth={1.9} aria-hidden="true" />;
      if (kind === "skill") return <BookOpenText size={14} strokeWidth={1.9} aria-hidden="true" />;
      if (kind === "terminal") return <TerminalSquare size={14} strokeWidth={1.9} aria-hidden="true" />;
      return <Wrench size={14} strokeWidth={1.9} aria-hidden="true" />;
  }
}

export function AwakenToolTraceLine({
  title,
  detail,
  output,
  outputLabel,
  outputTruncated = false,
  detailCard,
  status = "done",
  kind = "tool",
  icon,
  defaultOpen,
  open,
  onOpenChange,
  onCopyDetail,
  animateEntry = true,
  onSelect,
}: AwakenToolTraceLineProps) {
  const outputId = useId();
  const hasDetail = Boolean(detailCard?.sections.length || output);
  const detailLabel =
    detailCard?.title || outputLabel || (kind === "terminal" ? "终端输出" : "工具输出");
  const autoExpanded = false;
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const expanded = hasDetail && (open ?? manualExpanded ?? autoExpanded);
  const interactive = hasDetail || Boolean(onSelect);
  const lineClassName = `tool-trace-line tool-trace-line-${status} tool-trace-line-${kind}${
    expanded ? " tool-trace-line-open" : ""
  }`;

  function activate() {
    if (hasDetail) {
      const nextOpen = !(open ?? manualExpanded ?? autoExpanded);
      if (open === undefined) setManualExpanded(nextOpen);
      onOpenChange?.(nextOpen);
    }
    onSelect?.();
  }

  const rowContent = (
    <>
      <span className="tool-trace-line-icon">{icon ?? statusIcon(status, kind)}</span>
      <span className="tool-trace-line-copy">
        <span className="tool-trace-line-title">{title}</span>
        {detail ? <span className="tool-trace-line-detail">{detail}</span> : null}
      </span>
      <span
        className="tool-trace-line-chevron"
        aria-hidden="true"
        data-visible={hasDetail ? "available" : "empty"}
      >
        {hasDetail ? (
          <ChevronRight size={13} strokeWidth={1.8} />
        ) : null}
      </span>
    </>
  );

  return (
    <article
      className="tool-trace-item"
      data-status={status}
      data-kind={kind}
      data-animate-entry={animateEntry ? "true" : "false"}
    >
      {interactive ? (
        <button
          type="button"
          className={lineClassName}
          onClick={activate}
          aria-expanded={hasDetail ? expanded : undefined}
          aria-controls={hasDetail ? outputId : undefined}
        >
          {rowContent}
        </button>
      ) : (
        <div className={lineClassName} role="group">
          {rowContent}
        </div>
      )}
      {hasDetail ? (
        <div
          id={outputId}
          className="tool-trace-output-disclosure"
          data-expanded={expanded ? "true" : "false"}
          aria-hidden={!expanded}
          inert={!expanded}
        >
          <div className="tool-trace-output-rail">
            <section
              className="tool-trace-output-card"
              aria-label={detailLabel}
              data-output-truncated={outputTruncated ? "true" : "false"}
            >
              {outputTruncated || (detailCard?.copyText && onCopyDetail) ? (
                <div className="tool-trace-output-actions">
                  {outputTruncated ? (
                    <span className="tool-trace-output-truncated">内容已截断</span>
                  ) : null}
                  {detailCard?.copyText && onCopyDetail ? (
                    <button
                      type="button"
                      className="tool-trace-output-copy"
                      aria-label="复制详情"
                      onClick={() => onCopyDetail(detailCard.copyText || "")}
                    >
                      <Copy size={13} strokeWidth={1.8} aria-hidden="true" />
                    </button>
                  ) : null}
                </div>
              ) : null}
              {detailCard ? (
                <div
                  className="tool-trace-detail-sections"
                  data-aw-run-stage-scroll-boundary=""
                  tabIndex={0}
                >
                  {detailCard.sections.map((section, index) => (
                    <section
                      key={`${section.label}:${index}`}
                      className="tool-trace-detail-section"
                      data-format={section.format}
                      data-tone={section.tone || "default"}
                    >
                      <div className="tool-trace-detail-label">{section.label}</div>
                      <pre>{section.content}</pre>
                    </section>
                  ))}
                </div>
              ) : (
                <pre
                  className="tool-trace-output-scroll"
                  data-aw-run-stage-scroll-boundary=""
                  tabIndex={0}
                >
                  {output}
                </pre>
              )}
            </section>
          </div>
        </div>
      ) : null}
    </article>
  );
}
