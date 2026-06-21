import { AlertCircle, Check, LoaderCircle } from "lucide-react";

export type ToolActionStatus = "active" | "done" | "error";

type ToolActionRowProps = {
  title: string;
  detail?: string;
  status?: ToolActionStatus;
  onSelect?: () => void;
};

const statusTone: Record<ToolActionStatus, { border: string; background: string; color: string }> = {
  active: {
    border: "color-mix(in srgb, var(--aw-status-running) 24%, transparent)",
    background: "color-mix(in srgb, var(--aw-status-running) 10%, transparent)",
    color: "var(--aw-status-running)",
  },
  done: {
    border: "var(--aw-success-soft-border)",
    background: "var(--aw-success-soft-bg)",
    color: "var(--aw-status-success)",
  },
  error: {
    border: "var(--aw-error-soft-border)",
    background: "var(--aw-error-soft-bg)",
    color: "var(--aw-status-danger)",
  },
};

function statusIcon(status: ToolActionStatus) {
  if (status === "active") {
    return <LoaderCircle size={14} strokeWidth={2.2} style={{ animation: "spin 1s linear infinite" }} />;
  }
  if (status === "error") {
    return <AlertCircle size={14} strokeWidth={2.2} />;
  }
  return <Check size={14} strokeWidth={2.4} />;
}

export function ToolActionRow({
  title,
  detail,
  status = "done",
  onSelect,
}: ToolActionRowProps) {
  const tone = statusTone[status];

  return (
    <button
      type="button"
      className="tool-action-row"
      onClick={onSelect}
      style={{
        display: "grid",
        gridTemplateColumns: "auto minmax(0, 1fr)",
        gap: "10px",
        alignItems: "start",
        width: "100%",
        minWidth: 0,
        padding: "8px",
        border: "1px solid transparent",
        borderRadius: "var(--aw-radius-control)",
        background: "transparent",
        color: "inherit",
        textAlign: "left",
        cursor: onSelect ? "pointer" : "default",
        outline: "none",
      }}
    >
      <span
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          minWidth: "46px",
          height: "22px",
          padding: "0 7px",
          border: `1px solid ${tone.border}`,
          borderRadius: "var(--aw-radius-control)",
          background: tone.background,
          color: tone.color,
          fontSize: "var(--aw-font-size-xs)",
          fontWeight: "var(--aw-font-weight-semibold)",
          lineHeight: "var(--aw-line-height-xs)",
          whiteSpace: "nowrap",
        }}
      >
        {statusIcon(status)}
      </span>
      <span style={{ display: "grid", gap: "2px", minWidth: 0 }}>
        <strong
          style={{
            minWidth: 0,
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-sm)",
            lineHeight: "var(--aw-line-height-sm)",
            overflowWrap: "anywhere",
          }}
        >
          {title}
        </strong>
        {detail ? (
          <em
            style={{
              minWidth: 0,
              color: "var(--aw-text-subtle)",
              fontSize: "var(--aw-font-size-xs)",
              fontStyle: "normal",
              lineHeight: "var(--aw-line-height-xs)",
              overflowWrap: "anywhere",
            }}
          >
            {detail}
          </em>
        ) : null}
      </span>
    </button>
  );
}
