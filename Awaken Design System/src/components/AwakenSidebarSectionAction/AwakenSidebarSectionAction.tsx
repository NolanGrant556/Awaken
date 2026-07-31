import "./AwakenSidebarSectionAction.css";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { AwakenTooltip } from "../AwakenTooltip";

export type AwakenSidebarSectionActionProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  children: ReactNode;
};

export function AwakenSidebarSectionAction({
  label,
  children,
  ...props
}: AwakenSidebarSectionActionProps) {
  const button = (
    <button
      type="button"
      aria-label={label}
      {...props}
      className={`aw-sidebar-section-action${props.className ? ` ${props.className}` : ""}`}
    >
      {children}
    </button>
  );

  return <AwakenTooltip label={label}>{button}</AwakenTooltip>;
}
