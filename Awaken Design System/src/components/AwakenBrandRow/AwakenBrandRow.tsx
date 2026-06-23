import "./AwakenBrandRow.css";
import type { CSSProperties } from "react";
import logo from "../../assets/logo.svg";

export type AwakenBrandRowProps = {
  logoSrc?: string;
  title?: string;
  meta?: string;
};

const brandRowStyle: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "28px minmax(0, 1fr)",
  gap: 8,
  alignItems: "center",
  minHeight: 32,
  padding: "0 2px",
};

const brandMarkStyle: CSSProperties = {
  display: "grid",
  width: 28,
  height: 28,
  placeItems: "center",
  border: "1px solid var(--aw-border-main)",
  borderRadius: "var(--aw-radius-sm)",
  background: "var(--aw-button-black)",
  color: "var(--aw-text-primary)",
};

const brandCopyStyle: CSSProperties = {
  minWidth: 0,
};

const brandNameStyle: CSSProperties = {
  display: "block",
  overflow: "hidden",
  fontSize: "var(--aw-font-size-base)",
  fontWeight: "var(--aw-font-weight-semibold)",
  lineHeight: "var(--aw-line-height-sm)",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const brandMetaStyle: CSSProperties = {
  display: "block",
  overflow: "hidden",
  color: "var(--aw-text-tertiary)",
  fontSize: "var(--aw-font-size-xs)",
  lineHeight: "var(--aw-line-height-xs)",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

export function AwakenBrandRow({ logoSrc, title = "Awaken", meta }: AwakenBrandRowProps) {
  const src = logoSrc ?? logo;
  return (
    <div className="aw-brand-row" style={brandRowStyle}>
      <div style={brandMarkStyle} aria-hidden="true">
        <img src={src} alt="" width={16} height={16} style={{ filter: "invert(1)" }} />
      </div>
      <div style={brandCopyStyle}>
        <span style={brandNameStyle}>{title}</span>
        {meta ? <span style={brandMetaStyle}>{meta}</span> : null}
      </div>
    </div>
  );
}
