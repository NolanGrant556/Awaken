import "./AwakenProfileButton.css";
import type { CSSProperties, MouseEvent } from "react";
import { AwakenAvatar } from "../AwakenAvatar";
import { AwakenIcon } from "../AwakenIcon";

export type AwakenProfileButtonProps = {
  name: string;
  subtitle?: string;
  avatarFallback: string;
  avatarSrc?: string;
  onClick?: () => void;
};

const buttonStyle: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "32px minmax(0, 1fr) auto",
  gap: 8,
  alignItems: "center",
  width: "100%",
  minHeight: 44,
  padding: "6px 8px",
  border: 0,
  borderRadius: "var(--aw-radius-sm)",
  background: "transparent",
  color: "var(--aw-text-primary)",
  textAlign: "left",
  cursor: "pointer",
  transition: "background-color 160ms ease",
};

const nameStyle: CSSProperties = {
  display: "block",
  overflow: "hidden",
  color: "var(--aw-text-primary)",
  fontSize: "var(--aw-font-size-sm)",
  fontWeight: "var(--aw-font-weight-semibold)",
  lineHeight: "var(--aw-line-height-base)",
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
  onClick,
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
      type="button"
      onClick={onClick}
      aria-label={`用户 ${name}`}
      style={buttonStyle}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      onMouseDown={handleDown}
      onMouseUp={handleUp}
    >
      <AwakenAvatar src={avatarSrc} fallback={avatarFallback} />
      <span style={{ minWidth: 0, display: "grid", alignContent: "center", minHeight: 32 }}>
        <span style={nameStyle}>{name}</span>
        {subtitle ? <span style={subtitleStyle}>{subtitle}</span> : null}
      </span>
      <span style={{ display: "inline-flex", color: "var(--aw-text-tertiary)" }}>
        <AwakenIcon size={16}>
          <path d="M6 15l6-6 6 6" />
        </AwakenIcon>
      </span>
    </button>
  );
}
