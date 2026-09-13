import "./AwakenToolTraceLine.css";
import { memo, useId, useState, type ReactNode } from "react";

import { AwakenIcon } from "../AwakenIcon";
import { useAwakenDisclosurePresence } from "../../hooks/useAwakenDisclosurePresence";
import { splitAwakenTraceTitle } from "../../splitAwakenTraceTitle";
import { AwakenTraceTitle } from "../AwakenTraceTitle";
import { TraceDiff } from "./TraceDiff";

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
  /** Kept for API compatibility; the detail card no longer renders a copy control. */
  onCopyDetail?: (copyText: string) => void;
  /** Disable the mount animation when a consumer reveals already-finished history. */
  animateEntry?: boolean;
  onSelect?: () => void;
  fileLabel?: string;
  onOpenFile?: () => void;
};

export const AwakenToolTraceLine = memo(function AwakenToolTraceLine({
  title,
  detail,
  output,
  outputLabel,
  outputTruncated = false,
  detailCard,
  status = "done",
  kind = "tool",
  defaultOpen,
  open,
  onOpenChange,
  animateEntry = true,
  onSelect,
  fileLabel,
  onOpenFile,
}: AwakenToolTraceLineProps) {
  const outputId = useId();
  const hasDetail = Boolean(detailCard?.sections.length || output);
  const detailLabel =
    detailCard?.title ||
    outputLabel ||
    (kind === "terminal" ? "终端输出" : "工具输出");
  const autoExpanded = false;
  const [manualExpanded, setManualExpanded] = useState<boolean | null>(
    defaultOpen ?? null,
  );
  const expanded = hasDetail && (open ?? manualExpanded ?? autoExpanded);
  const {
    present: detailPresent,
    prepareOpen,
    onTransitionEnd,
  } = useAwakenDisclosurePresence(expanded, 180);
  const interactive = hasDetail || Boolean(onSelect);
  const lineClassName = `tool-trace-line tool-trace-line-${status} tool-trace-line-${kind}${
    expanded ? " tool-trace-line-open" : ""
  }`;

  function activate() {
    if (hasDetail) {
      const nextOpen = !(open ?? manualExpanded ?? autoExpanded);
      if (nextOpen) prepareOpen();
      if (open === undefined) setManualExpanded(nextOpen);
      onOpenChange?.(nextOpen);
    }
    onSelect?.();
  }

  const { verb, fact } = splitAwakenTraceTitle(title);
  const titleNode = <AwakenTraceTitle title={title} />;
  const fileFact = fileLabel || fact || verb;

  const rowContent = (
    <>
      <span className="tool-trace-line-copy">
        <span className="tool-trace-line-title">{titleNode}</span>
        {detail ? (
          <span className="tool-trace-line-detail">{detail}</span>
        ) : null}
      </span>
      {status === "active" ? (
        <span className="aw-trace-dots" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
      ) : null}
      <span
        className="tool-trace-line-chevron"
        aria-hidden="true"
        data-visible={hasDetail ? "available" : "empty"}
      >
        {hasDetail ? <AwakenIcon concept="disclose" size={14} /> : null}
      </span>
    </>
  );

  return (
    <article
      className="tool-trace-item"
      data-status={status}
      data-kind={kind}
      data-animate-entry={animateEntry ? "true" : "false"}
      data-disclosure-state={
        expanded ? "open" : detailPresent ? "closing" : "closed"
      }
    >
      {onOpenFile ? (
        <div className={`${lineClassName} tool-trace-file-row`}>
          <span className="tool-trace-line-copy">
            <span className="tool-trace-line-title">
              <span className="aw-trace-title">
                <span className="aw-trace-verb">{verb}</span>
                {fact || fileLabel ? (
                  <>
                    {" "}
                    <button
                      type="button"
                      className="tool-trace-file-link aw-trace-fact"
                      title="打开当前磁盘文件"
                      aria-label={`打开文件 ${fileFact}`}
                      onClick={onOpenFile}
                    >
                      {fileFact}
                    </button>
                  </>
                ) : null}
              </span>
            </span>
          </span>
          {detail ? (
            <span className="tool-trace-line-detail">{detail}</span>
          ) : null}
          {status === "active" ? (
            <span className="aw-trace-dots" aria-hidden="true">
              <i />
              <i />
              <i />
            </span>
          ) : null}
          {hasDetail ? (
            <button
              type="button"
              className="tool-trace-detail-toggle"
              aria-label={`查看 ${fileLabel || title} 的执行详情`}
              aria-expanded={expanded}
              aria-controls={outputId}
              onClick={activate}
            >
              <AwakenIcon concept="disclose" size={14} />
            </button>
          ) : null}
        </div>
      ) : interactive ? (
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
          onTransitionEnd={onTransitionEnd}
        >
          {detailPresent ? (
            <div className="tool-trace-output-rail">
              <section
                className="tool-trace-output-card"
                aria-label={detailLabel}
                data-output-truncated={outputTruncated ? "true" : "false"}
              >
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
                        {section.format === "json" ? (
                          <details>
                            <summary className="tool-trace-detail-label">
                              {section.label}
                            </summary>
                            <pre>{section.content}</pre>
                          </details>
                        ) : section.format === "diff" ? (
                          <TraceDiff patch={section.content} />
                        ) : (
                          <>
                            <div className="tool-trace-detail-label">
                              {section.label}
                            </div>
                            <pre>{section.content}</pre>
                          </>
                        )}
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
                {outputTruncated ? (
                  <span className="tool-trace-output-truncated">内容已截断</span>
                ) : null}
              </section>
            </div>
          ) : null}
        </div>
      ) : null}
    </article>
  );
});
