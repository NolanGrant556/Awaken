import { useEffect } from "react";

type ToastProps = {
  message: string;
  visible: boolean;
  duration?: number;
  onClose?: () => void;
};

export function Toast({
  message,
  visible,
  duration = 1500,
  onClose,
}: ToastProps) {
  useEffect(() => {
    if (!visible) return;
    const timer = setTimeout(() => {
      onClose?.();
    }, duration);
    return () => clearTimeout(timer);
  }, [visible, duration, onClose]);

  if (!visible) return null;

  return (
    <div
      role="status"
      aria-live="polite"
      style={{
        position: "fixed",
        right: "24px",
        bottom: "24px",
        zIndex: 20,
        maxWidth: "320px",
        padding: "12px 16px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-md)",
        background: "var(--aw-bg-white)",
        color: "var(--aw-text-primary)",
        boxShadow: "var(--aw-shadow-toast)",
        fontSize: "var(--aw-font-size-sm)",
        lineHeight: "var(--aw-line-height-sm)",
        animation: "panel-in 220ms ease-out",
        pointerEvents: "none",
      }}
    >
      {message}
    </div>
  );
}
