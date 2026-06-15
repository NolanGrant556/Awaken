import type { ButtonHTMLAttributes, ReactNode } from "react";

type IconButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  children: ReactNode;
};

export function IconButton({ label, children, className = "", ...props }: IconButtonProps) {
  return (
    <button className={`icon-button ${className}`} aria-label={label} title={label} {...props}>
      {children}
    </button>
  );
}

type UiButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  icon?: ReactNode;
  children: ReactNode;
};

export function UiButton({ icon, children, className = "", ...props }: UiButtonProps) {
  return (
    <button className={`ui-button ${className}`} {...props}>
      {icon}
      <span>{children}</span>
    </button>
  );
}

type StatusPillProps = {
  tone?: "local" | "permission" | "neutral";
  icon?: ReactNode;
  children: ReactNode;
};

export function StatusPill({ tone = "neutral", icon, children }: StatusPillProps) {
  return (
    <span className={`status-pill status-pill-${tone}`}>
      {icon}
      <span>{children}</span>
    </span>
  );
}
