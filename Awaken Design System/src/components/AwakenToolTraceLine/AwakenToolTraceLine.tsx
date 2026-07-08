import "./AwakenToolTraceLine.css";
import { AlertCircle, Check, LoaderCircle } from "lucide-react";
import { useId, useState, type ReactNode } from "react";

export type ToolTraceStatus = "active" | "done" | "error";

export type AwakenToolTraceLineProps = {
  title: string;
  detail?: string;
  status?: ToolTraceStatus;
  icon?: ReactNode;
  onSelect?: () => void;
};

const COLLAPSIBLE_DETAIL_CHAR_LIMIT = 420;
const COLLAPSIBLE_DETAIL_LINE_LIMIT = 6;

function statusIcon(status: ToolTraceStatus) {
  if (status === "active") {
    return <LoaderCircle size={14} strokeWidth={2.1} aria-hidden="true" />;
  }
  if (status === "error") {
    return <AlertCircle size={14} strokeWidth={2.1} aria-hidden="true" />;
  }
  return <Check size={14} strokeWidth={2.25} aria-hidden="true" />;
}

function shouldCollapseDetail(detail?: string): boolean {
  if (!detail) {
    return false;
  }
  return (
    detail.length > COLLAPSIBLE_DETAIL_CHAR_LIMIT ||
    detail.split(/\r\n|\r|\n/).length > COLLAPSIBLE_DETAIL_LINE_LIMIT
  );
}

export function AwakenToolTraceLine({
  title,
  detail,
  status = "done",
  icon,
  onSelect,
}: AwakenToolTraceLineProps) {
  const interactive = Boolean(onSelect);
  const [detailExpanded, setDetailExpanded] = useState(false);
  const detailId = useId();
  const collapsibleDetail = shouldCollapseDetail(detail);
  const lineClassName = `tool-trace-line tool-trace-line-${status}${
    collapsibleDetail ? " tool-trace-line-has-collapsible-detail" : ""
  }`;
  const copyClassName = `tool-trace-line-copy${
    collapsibleDetail ? " tool-trace-line-copy-long" : ""
  }`;

  const detailContent = detail ? (
    <span
      id={collapsibleDetail ? detailId : undefined}
      className={`tool-trace-line-detail${
        collapsibleDetail ? " tool-trace-line-detail-long" : ""
      }${
        collapsibleDetail && !detailExpanded
          ? " tool-trace-line-detail-collapsed"
          : ""
      }`}
    >
      {detail}
    </span>
  ) : null;

  const detailToggle = collapsibleDetail ? (
    <button
      type="button"
      className="tool-trace-line-detail-toggle"
      aria-expanded={detailExpanded}
      aria-controls={detailId}
      onClick={() => setDetailExpanded((expanded) => !expanded)}
    >
      {detailExpanded ? "收起返回" : "展开完整返回"}
    </button>
  ) : null;

  const content = (
    <>
      <span className="tool-trace-line-icon">{icon ?? statusIcon(status)}</span>
      <span className={copyClassName}>
        <span className="tool-trace-line-title">{title}</span>
        {detailContent}
        {detailToggle}
      </span>
    </>
  );

  if (interactive && !collapsibleDetail) {
    return (
      <button
        type="button"
        className={lineClassName}
        onClick={onSelect}
      >
        {content}
      </button>
    );
  }

  if (interactive) {
    return (
      <div className={lineClassName} role="group">
        <span className="tool-trace-line-icon">{icon ?? statusIcon(status)}</span>
        <span className={copyClassName}>
          <button
            type="button"
            className="tool-trace-line-title-button"
            onClick={onSelect}
          >
            <span className="tool-trace-line-title">{title}</span>
          </button>
          {detailContent}
          {detailToggle}
        </span>
      </div>
    );
  }

  return (
    <div className={lineClassName} role="group">
      {content}
    </div>
  );
}
