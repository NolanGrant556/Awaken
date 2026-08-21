import "./AwakenSendButton.css";
import { ArrowUp, Square } from "../../icons";
import { AwakenTooltip } from "../AwakenTooltip";

export type AwakenSendButtonProps = {
  ready: boolean;
  onClick?: () => void;
  disabled?: boolean;
  loading?: boolean;
  mode?: "send" | "stop";
};

export function AwakenSendButton({ ready, onClick, disabled, loading, mode = "send" }: AwakenSendButtonProps) {
  const isStop = mode === "stop";
  const label = isStop ? "停止运行" : loading ? "发送中" : "发送";
  const isDisabled = disabled || loading;

  const button = (
    <button
      type="button"
      onClick={onClick}
      disabled={isDisabled}
      aria-label={label}
      title={label}
      style={{
        display: "inline-grid",
        placeItems: "center",
        width: 32,
        height: 32,
        padding: 0,
        border: "none",
        borderRadius: "var(--aw-radius-pill)",
        background: isStop
          ? "var(--aw-status-danger)"
          : ready
            ? "var(--aw-button-black)"
            : "var(--aw-bg-fill-dark)",
        color: ready || isStop ? "var(--aw-text-onblack)" : "var(--aw-text-disabled)",
        cursor: isDisabled ? "not-allowed" : "pointer",
        opacity: isDisabled ? 0.45 : 1,
        transition: "background-color var(--aw-transition-fast), color var(--aw-transition-fast), opacity var(--aw-transition-fast)",
      }}
      onMouseEnter={(e) => {
        if (!isDisabled && (ready || isStop)) {
          e.currentTarget.style.background = isStop
            ? "var(--aw-status-danger-text)"
            : "var(--aw-button-black-hover)";
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = isStop
          ? "var(--aw-status-danger)"
          : ready
            ? "var(--aw-button-black)"
            : "var(--aw-bg-fill-dark)";
      }}
    >
      {loading ? (
        <span
          style={{
            width: 14,
            height: 14,
            border: "2px solid currentColor",
            borderTopColor: "transparent",
            borderRadius: "var(--aw-radius-pill)",
            animation: "spin 900ms linear infinite",
          }}
        />
      ) : isStop ? (
        <Square size={14} strokeWidth={2.2} fill="currentColor" />
      ) : (
        <ArrowUp size={18} strokeWidth={1.85} />
      )}
    </button>
  );

  return <AwakenTooltip label={label}>{button}</AwakenTooltip>;
}
