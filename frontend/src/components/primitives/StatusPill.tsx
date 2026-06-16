import type { ReactNode } from "react";
import type { ComponentTone } from "../../types";

type StatusPillProps = {
  tone: ComponentTone;
  icon?: ReactNode;
  children: ReactNode;
};

const toneStyles: Record<ComponentTone, React.CSSProperties> = {
  neutral: {
    borderColor: "var(--aw-border-main)",
    background: "var(--aw-bg-fill-light)",
    color: "var(--aw-text-secondary)",
  },
  local: {
    borderColor: "var(--aw-success-soft-border)",
    background: "var(--aw-success-soft-bg)",
    color: "var(--aw-success)",
  },
  permission: {
    borderColor: "var(--aw-warning-soft-border)",
    background: "var(--aw-warning-soft-bg)",
    color: "var(--aw-warning)",
  },
  success: {
    borderColor: "var(--aw-success-soft-border)",
    background: "var(--aw-success-soft-bg)",
    color: "var(--aw-success)",
  },
  warning: {
    borderColor: "var(--aw-warning-soft-border)",
    background: "var(--aw-warning-soft-bg)",
    color: "var(--aw-warning)",
  },
  error: {
    borderColor: "var(--aw-error-soft-border)",
    background: "var(--aw-error-soft-bg)",
    color: "var(--aw-error)",
  },
};

export function StatusPill({ tone, icon, children }: StatusPillProps) {
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        gap: "4px",
        height: 28,
        padding: "0 10px",
        border: "1px solid",
        borderRadius: "var(--aw-radius-sm)",
        fontSize: "var(--aw-font-size-sm)",
        fontWeight: "var(--aw-font-weight-heavy)",
        whiteSpace: "nowrap",
        ...toneStyles[tone],
      }}
    >
      {icon}
      <span>{children}</span>
    </span>
  );
}
