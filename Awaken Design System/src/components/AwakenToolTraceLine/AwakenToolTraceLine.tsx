import "./AwakenToolTraceLine.css";
import { AlertCircle, Check, LoaderCircle } from "lucide-react";
import type { KeyboardEvent, ReactNode } from "react";

export type ToolTraceStatus = "active" | "done" | "error";

export type AwakenToolTraceLineProps = {
  title: string;
  detail?: string;
  status?: ToolTraceStatus;
  icon?: ReactNode;
  onSelect?: () => void;
};

function statusIcon(status: ToolTraceStatus) {
  if (status === "active") {
    return <LoaderCircle size={14} strokeWidth={2.1} aria-hidden="true" />;
  }
  if (status === "error") {
    return <AlertCircle size={14} strokeWidth={2.1} aria-hidden="true" />;
  }
  return <Check size={14} strokeWidth={2.25} aria-hidden="true" />;
}

export function AwakenToolTraceLine({
  title,
  detail,
  status = "done",
  icon,
  onSelect,
}: AwakenToolTraceLineProps) {
  const interactive = Boolean(onSelect);
  const content = (
    <>
      <span className="tool-trace-line-icon">{icon ?? statusIcon(status)}</span>
      <span className="tool-trace-line-copy">
        <span className="tool-trace-line-title">{title}</span>
        {detail ? <span className="tool-trace-line-detail">{detail}</span> : null}
      </span>
    </>
  );

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!interactive || (event.key !== "Enter" && event.key !== " ")) {
      return;
    }
    event.preventDefault();
    onSelect?.();
  };

  if (interactive) {
    return (
      <button
        type="button"
        className={`tool-trace-line tool-trace-line-${status}`}
        onClick={onSelect}
      >
        {content}
      </button>
    );
  }

  return (
    <div
      className={`tool-trace-line tool-trace-line-${status}`}
      role="group"
      tabIndex={interactive ? 0 : undefined}
      onKeyDown={handleKeyDown}
    >
      {content}
    </div>
  );
}
