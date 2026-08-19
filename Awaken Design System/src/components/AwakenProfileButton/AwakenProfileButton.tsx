import "./AwakenProfileButton.css";
import type { CSSProperties, MouseEvent, Ref } from "react";
import { ChevronUp } from "lucide-react";
import { AwakenAvatar } from "../AwakenAvatar";

export type AwakenProfileButtonProps = {
  name: string;
  subtitle?: string;
  avatarFallback: string;
  avatarSrc?: string;
  expanded?: boolean;
  controls?: string;
  onClick?: () => void;
  buttonRef?: Ref<HTMLButtonElement>;
};

const buttonStyle: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "30px minmax(0, 1fr) auto",
  gap: 10,
  alignItems: "center",
  width: "100%",
  minHeight: 44,
  padding: "6px 10px",
  border: 0,
  borderRadius: "var(--aw-radius-md)",
  background: "transparent",
  color: "var(--aw-text-primary)",
  textAlign: "left",
  cursor: "pointer",
  transition: "background-color var(--aw-transition-fast)",
};

const nameStyle: CSSProperties = {
  display: "block",
  overflow: "hidden",
  color: "var(--aw-text-primary)",
  fontSize: "var(--aw-font-size-sm)",
  fontWeight: "var(--aw-font-weight-semibold)",
  lineHeight: "var(--aw-line-height-sm)",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const subtitleStyle: CSSProperties = {
  display: "block",
  overflow: "hidden",
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

export function AwakenProfileButton({
  name,
  subtitle,
  avatarFallback,
  avatarSrc,
  expanded,
  controls,
  onClick,
  buttonRef,
}: AwakenProfileButtonProps) {
  const handleEnter = (e: MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = "var(--aw-bg-fill-light)";
  };
  const handleLeave = (e: MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = "transparent";
  };
  const handleDown = (e: MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = "var(--aw-bg-fill-main)";
  };
  const handleUp = (e: MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = "var(--aw-bg-fill-light)";
  };

  return (
    <button
      ref={buttonRef}
      type="button"
      onClick={onClick}
      aria-label={`用户 ${name}`}
      aria-haspopup="dialog"
      aria-expanded={expanded}
      aria-controls={controls}
      style={buttonStyle}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      onMouseDown={handleDown}
      onMouseUp={handleUp}
    >
      <AwakenAvatar src={avatarSrc} fallback={avatarFallback} />
      <span style={{ minWidth: 0, display: "grid", alignContent: "center", minHeight: 30 }}>
        <span style={nameStyle}>{name}</span>
        {subtitle ? <span style={subtitleStyle}>{subtitle}</span> : null}
      </span>
      <span style={{ display: "inline-flex", color: "var(--aw-text-tertiary)" }}>
        <ChevronUp size={16} strokeWidth={1.85} aria-hidden="true" />
      </span>
    </button>
  );
}
