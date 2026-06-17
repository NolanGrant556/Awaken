import { ArrowUp } from "lucide-react";

type SendButtonProps = {
  ready: boolean;
  onClick?: () => void;
  disabled?: boolean;
  loading?: boolean;
};

export function SendButton({ ready, onClick, disabled, loading }: SendButtonProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled || loading}
      aria-label={loading ? "发送中" : "发送"}
      title={loading ? "发送中" : "发送"}
      style={{
        display: "inline-grid",
        placeItems: "center",
        width: 32,
        height: 32,
        border: "none",
        borderRadius: "999px",
        background: ready ? "var(--aw-button-black)" : "var(--aw-bg-fill-dark)",
        color: "var(--aw-text-onblack)",
        cursor: disabled || loading ? "not-allowed" : "pointer",
        opacity: disabled || loading ? 0.45 : 1,
        transition: "background-color 160ms ease, transform 160ms ease",
      }}
      onMouseEnter={(e) => {
        if (!disabled && !loading && ready) {
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
            borderRadius: "999px",
            animation: "spin 900ms linear infinite",
          }}
        />
      ) : (
        <ArrowUp size={18} strokeWidth={1.85} />
      )}
    </button>
  );
}
