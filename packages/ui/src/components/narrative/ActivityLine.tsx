import { AlertCircle, Check, LoaderCircle } from "lucide-react";

type ActivityLineStatus = "active" | "done" | "error";

type ActivityLineProps = {
  title: string;
  detail?: string;
  status?: ActivityLineStatus;
};

const statusStyles: Record<
  ActivityLineStatus,
  { background: string; border: string; color: string }
> = {
  active: {
    background: "color-mix(in srgb, var(--aw-status-running) 10%, transparent)",
    border: "color-mix(in srgb, var(--aw-status-running) 24%, transparent)",
    color: "var(--aw-status-running)",
  },
  done: {
    background: "var(--aw-success-soft-bg)",
    border: "var(--aw-success-soft-border)",
    color: "var(--aw-status-success)",
  },
  error: {
    background: "var(--aw-error-soft-bg)",
    border: "var(--aw-error-soft-border)",
    color: "var(--aw-status-danger)",
  },
};

export function ActivityLine({
  title,
  detail,
  status = "active",
}: ActivityLineProps) {
  const tone = statusStyles[status];
  const icon =
    status === "active" ? (
      <LoaderCircle size={15} strokeWidth={2.2} style={{ animation: "spin 1s linear infinite" }} />
    ) : status === "error" ? (
      <AlertCircle size={15} strokeWidth={2.2} />
    ) : (
      <Check size={15} strokeWidth={2.4} />
    );

  return (
    <div
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: "10px",
        width: "100%",
        maxWidth: "100%",
        boxSizing: "border-box",
        minWidth: 0,
        padding: "12px 14px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-panel)",
        background: "var(--aw-surface-raised)",
      }}
      aria-live={status === "active" ? "polite" : undefined}
    >
      <span
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          flex: "0 0 auto",
          width: "24px",
          height: "24px",
          border: `1px solid ${tone.border}`,
          borderRadius: "var(--aw-radius-pill)",
          background: tone.background,
          color: tone.color,
        }}
      >
        {icon}
      </span>
      <span
        style={{
          display: "grid",
          gap: "2px",
          minWidth: 0,
        }}
      >
        <strong
          style={{
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-sm)",
            fontWeight: "var(--aw-font-weight-bold)",
            lineHeight: "var(--aw-line-height-sm)",
            overflowWrap: "anywhere",
          }}
        >
          {title}
        </strong>
        {detail ? (
          <span
            style={{
              color: "var(--aw-text-subtle)",
              fontSize: "var(--aw-font-size-xs)",
              lineHeight: "var(--aw-line-height-xs)",
              overflowWrap: "anywhere",
            }}
          >
            {detail}
          </span>
        ) : null}
      </span>
    </div>
  );
}
