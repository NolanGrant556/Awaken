import "./AwakenToolTraceLine.css";
import {
  AlertCircle,
  Ban,
  BookOpenText,
  Check,
  ChevronRight,
  CircleSlash,
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

export type AwakenToolTraceLineProps = {
  title: string;
  /** Compact one-line context shown beside the title. */
  detail?: string;
  /** Bounded command or tool result rendered in the nested output card. */
  output?: string;
  outputLabel?: string;
  outputTruncated?: boolean;
  status?: ToolTraceStatus;
  kind?: ToolTraceKind;
  icon?: ReactNode;
  defaultOpen?: boolean;
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

function kindLabel(kind: ToolTraceKind): string | undefined {
  if (kind === "mcp") return "MCP";
  if (kind === "skill") return "Skill";
  if (kind === "terminal") return "Terminal";
  return undefined;
}

export function AwakenToolTraceLine({
  title,
  detail,
  output,
  outputLabel,
  outputTruncated = false,
  status = "done",
  kind = "tool",
  icon,
  defaultOpen,
  animateEntry = true,
  onSelect,
}: AwakenToolTraceLineProps) {
  const outputId = useId();
  const hasOutput = Boolean(output);
  const autoExpanded = status === "active" && hasOutput;
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const expanded = hasOutput && (manualExpanded ?? autoExpanded);
  const interactive = hasOutput || Boolean(onSelect);
  const category = kindLabel(kind);
  const lineClassName = `tool-trace-line tool-trace-line-${status} tool-trace-line-${kind}${
    expanded ? " tool-trace-line-open" : ""
  }`;

  function activate() {
    if (hasOutput) {
      setManualExpanded((current) => !(current ?? autoExpanded));
    }
    onSelect?.();
  }

  const rowContent = (
    <>
      <span className="tool-trace-line-icon">{icon ?? statusIcon(status, kind)}</span>
      <span className="tool-trace-line-copy">
        <span className="tool-trace-line-title">{title}</span>
        {category ? <span className="tool-trace-line-kind">{category}</span> : null}
        {detail ? <span className="tool-trace-line-detail">{detail}</span> : null}
      </span>
      {status === "done" ? (
        <span className="tool-trace-line-status" aria-label="已完成">
          <Check size={12} strokeWidth={2.2} aria-hidden="true" />
        </span>
      ) : null}
      {hasOutput ? (
        <span className="tool-trace-line-chevron" aria-hidden="true">
          <ChevronRight size={13} strokeWidth={1.8} />
        </span>
      ) : null}
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
          aria-expanded={hasOutput ? expanded : undefined}
          aria-controls={hasOutput ? outputId : undefined}
        >
          {rowContent}
        </button>
      ) : (
        <div className={lineClassName} role="group">
          {rowContent}
        </div>
      )}
      {hasOutput ? (
        <div
          id={outputId}
          className="tool-trace-output-disclosure"
          data-expanded={expanded ? "true" : "false"}
          aria-hidden={!expanded}
          inert={!expanded}
        >
          <div className="tool-trace-output-rail">
            <section className="tool-trace-output-card" aria-label={outputLabel || "工具输出"}>
              <div className="tool-trace-output-heading">
                <span>{outputLabel || (kind === "terminal" ? "终端输出" : "工具输出")}</span>
                {outputTruncated ? <span>内容已截断</span> : null}
              </div>
              <pre>{output}</pre>
            </section>
          </div>
        </div>
      ) : null}
    </article>
  );
}
