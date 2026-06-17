import type { ReactNode } from "react";
import type { PlatformKind } from "../../types";
import { Minus, Square, X } from "lucide-react";

type WindowTitleBarProps = {
  platform: PlatformKind;
  onMinimize?: () => void;
  onMaximize?: () => void;
  onClose?: () => void;
};

function TrafficDot({ color }: { color: string }) {
  return (
    <span
      style={{
        width: "12px",
        height: "12px",
        borderRadius: "999px",
        background: color,
        boxShadow: "inset 0 -1px 0 rgba(0, 0, 0, 0.12)",
      }}
    />
  );
}

function WinButton({
  label,
  onClick,
  children,
}: {
  label: string;
  onClick?: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={() => onClick?.()}
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        width: "34px",
        height: "28px",
        border: "none",
        borderRadius: "var(--aw-radius-sm)",
        background: "transparent",
        color: "var(--aw-text-secondary)",
        cursor: onClick ? "pointer" : "default",
        opacity: onClick ? 1 : 0.45,
        transition: "background-color 160ms ease, color 160ms ease",
      }}
      onMouseEnter={(e) => {
        if (onClick) {
          e.currentTarget.style.background = "var(--aw-bg-fill-light)";
          e.currentTarget.style.color = "var(--aw-text-primary)";
        }
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = "transparent";
        e.currentTarget.style.color = "var(--aw-text-secondary)";
      }}
    >
      {children}
    </button>
  );
}

export function WindowTitleBar({
  platform,
  onMinimize,
  onMaximize,
  onClose,
}: WindowTitleBarProps) {
  const barStyle: React.CSSProperties = {
    display: "flex",
    alignItems: "center",
    height: "38px",
    padding: "0 12px",
    background: "var(--aw-bg-nav)",
    borderBottom: "1px solid var(--aw-border-light)",
  };

  if (platform === "macos") {
    return (
      <div style={{ ...barStyle, gap: "8px" }}>
        <TrafficDot color="#ff5f57" />
        <TrafficDot color="#ffbd2e" />
        <TrafficDot color="#28c840" />
      </div>
    );
  }

  if (platform === "windows") {
    return (
      <div
        style={{
          ...barStyle,
          justifyContent: "flex-end",
          gap: "4px",
        }}
      >
        <WinButton label="最小化" onClick={onMinimize}>
          <Minus size={14} strokeWidth={1.85} />
        </WinButton>
        <WinButton label="最大化" onClick={onMaximize}>
          <Square size={12} strokeWidth={1.85} />
        </WinButton>
        <WinButton label="关闭" onClick={onClose}>
          <X size={14} strokeWidth={1.85} />
        </WinButton>
      </div>
    );
  }

  return null;
}
