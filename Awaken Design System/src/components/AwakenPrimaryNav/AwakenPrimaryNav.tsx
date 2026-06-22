import "./AwakenPrimaryNav.css";
import type { CSSProperties, MouseEvent } from "react";
import type { NavItem } from "../../types";

export type AwakenPrimaryNavProps = {
  items: NavItem[];
  activeId?: string;
  onSelect?: (id: string) => void;
};

const navStyle: CSSProperties = {
  display: "grid",
  gap: 2,
  padding: "2px 14px 12px",
};

const rowBase: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "20px minmax(0, 1fr) auto",
  alignItems: "center",
  width: "100%",
  height: 32,
  gap: 9,
  padding: "0 8px",
  border: 0,
  borderRadius: "var(--aw-radius-sm)",
  background: "transparent",
  color: "var(--aw-text-secondary)",
  fontSize: "var(--aw-font-size-base)",
  lineHeight: "var(--aw-line-height-base)",
  textAlign: "left",
  cursor: "pointer",
  transition: "background-color 160ms ease, color 160ms ease",
};

const iconWrapStyle: CSSProperties = {
  display: "inline-flex",
  width: 20,
  height: 20,
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
};

const labelStyle: CSSProperties = {
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const badgeStyle: CSSProperties = {
  minWidth: 18,
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
  textAlign: "right",
  whiteSpace: "nowrap",
};

export function AwakenPrimaryNav({ items, activeId, onSelect }: AwakenPrimaryNavProps) {
  return (
    <nav className="aw-primary-nav" aria-label="主导航" style={navStyle}>
      {items.map((item) => {
        const isActive = item.id === activeId;
        const isDisabled = item.disabled;
        const baseBg = isActive ? "var(--aw-bg-fill-main)" : "transparent";
        const baseColor = isActive
          ? "var(--aw-text-primary)"
          : isDisabled
            ? "var(--aw-text-disabled)"
            : "var(--aw-text-secondary)";

        const handleEnter = (e: MouseEvent<HTMLButtonElement>) => {
          if (!isDisabled) {
            e.currentTarget.style.background = isActive
              ? "var(--aw-bg-fill-main)"
              : "var(--aw-bg-fill-light)";
          }
        };
        const handleLeave = (e: MouseEvent<HTMLButtonElement>) => {
          e.currentTarget.style.background = baseBg;
        };
        const handleDown = (e: MouseEvent<HTMLButtonElement>) => {
          if (!isDisabled) e.currentTarget.style.background = "var(--aw-bg-fill-dark)";
        };
        const handleUp = (e: MouseEvent<HTMLButtonElement>) => {
          if (!isDisabled) {
            e.currentTarget.style.background = isActive
              ? "var(--aw-bg-fill-main)"
              : "var(--aw-bg-fill-light)";
          }
        };

        return (
          <button
            key={item.id}
            type="button"
            disabled={isDisabled}
            onClick={() => !isDisabled && onSelect?.(item.id)}
            style={{
              ...rowBase,
              background: baseBg,
              color: baseColor,
              cursor: isDisabled ? "not-allowed" : "pointer",
              opacity: isDisabled ? 0.6 : 1,
            }}
            onMouseEnter={handleEnter}
            onMouseLeave={handleLeave}
            onMouseDown={handleDown}
            onMouseUp={handleUp}
          >
            <span style={iconWrapStyle}>{item.icon ?? null}</span>
            <span style={labelStyle}>{item.label}</span>
            <span style={badgeStyle}>{item.badge ?? ""}</span>
          </button>
        );
      })}
    </nav>
  );
}
