import "./AwakenSendButton.css";
import { ArrowUp, Square } from "lucide-react";
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
        color: "var(--aw-text-onblack)",
        cursor: isDisabled ? "not-allowed" : "pointer",
        opacity: isDisabled ? 0.45 : 1,
        transition: "background-color 160ms ease, transform 160ms ease",
      }}
      onMouseEnter={(e) => {
        if (!isDisabled && (ready || isStop)) {
          e.currentTarget.style.transform = "translateY(-1px)";
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.transform = "translateY(0)";
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
