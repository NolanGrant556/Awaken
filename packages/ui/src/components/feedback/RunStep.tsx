import type { RunStepStatus } from "../../types";
import { Check, AlertCircle } from "lucide-react";

type RunStepProps = {
  index: number;
  title: string;
  description?: string;
  tag?: string;
  status: RunStepStatus;
};

const badgeSize = 22;

const statusBadgeStyles: Record<RunStepStatus, React.CSSProperties> = {
  pending: {
    background: "var(--aw-bg-fill-light)",
    color: "var(--aw-text-disabled)",
    animation: "none",
  },
  active: {
    background: "color-mix(in srgb, var(--aw-text-blue) 12%, transparent)",
    color: "var(--aw-text-blue)",
    animation: "pulse 1.5s cubic-bezier(0.4, 0, 0.6, 1) infinite",
  },
  done: {
    background: "var(--aw-success-soft-bg)",
    color: "var(--aw-success)",
    animation: "none",
  },
  error: {
    background: "var(--aw-error-soft-bg)",
    color: "var(--aw-error)",
    animation: "none",
  },
};

const titleColors: Record<RunStepStatus, string> = {
  pending: "var(--aw-text-secondary)",
  active: "var(--aw-text-primary)",
  done: "var(--aw-text-primary)",
  error: "var(--aw-text-primary)",
};

export function RunStep({ index, title, description, tag, status }: RunStepProps) {
  const badgeStyle: React.CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    width: badgeSize,
    height: badgeSize,
    flex: "0 0 auto",
    borderRadius: "999px",
    fontSize: "var(--aw-font-size-xs)",
    fontWeight: "var(--aw-font-weight-bold)",
    lineHeight: 1,
    ...statusBadgeStyles[status],
  };

  const badgeContent =
    status === "done" ? (
      <Check size={14} strokeWidth={2.5} />
    ) : status === "error" ? (
      <AlertCircle size={14} strokeWidth={2.5} />
    ) : (
      <span>{index}</span>
    );

  return (
    <div
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: "10px",
        padding: "7px 0",
      }}
    >
      <span style={badgeStyle}>{badgeContent}</span>
      <div
        style={{
          flex: "1 1 auto",
          minWidth: 0,
          display: "grid",
          gap: "2px",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "8px",
            minWidth: 0,
          }}
        >
          <span
            className="aw-truncate"
            style={{
              fontSize: "var(--aw-font-size-sm)",
              lineHeight: "var(--aw-line-height-sm)",
              fontWeight: "var(--aw-font-weight-semibold)",
              color: titleColors[status],
            }}
            title={title}
          >
            {title}
          </span>
          {tag ? (
            <span
              className="aw-truncate"
              style={{
                flex: "0 1 auto",
                minWidth: 0,
                padding: "0 6px",
                borderRadius: "var(--aw-radius-sm)",
                background: "var(--aw-bg-fill-light)",
                color: "var(--aw-text-tertiary)",
                fontSize: "var(--aw-font-size-xs)",
                fontFamily: "var(--aw-font-mono)",
                lineHeight: "var(--aw-line-height-xs)",
              }}
              title={tag}
            >
              {tag}
            </span>
          ) : null}
        </div>
        {description ? (
          <span
            className="aw-truncate"
            style={{
              fontSize: "var(--aw-font-size-xs)",
              lineHeight: "var(--aw-line-height-xs)",
              color:
                status === "error"
                  ? "var(--aw-error)"
                  : "var(--aw-text-tertiary)",
            }}
            title={description}
          >
            {description}
          </span>
        ) : null}
      </div>
    </div>
  );
}
