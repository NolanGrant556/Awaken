import type { ReactNode } from "react";
import type { PlatformKind } from "../../types";

type KbdProps = {
  children: ReactNode;
  platform?: PlatformKind;
};

export function Kbd({ children, platform = "macos" }: KbdProps) {
  const isMac = platform === "macos";
  const text = typeof children === "string" ? children : "";
  const display = text.startsWith("⌘") || text.startsWith("Ctrl+")
    ? text
    : `${isMac ? "⌘" : "Ctrl+"}${text}`;

  return (
    <kbd
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        minWidth: 18,
        height: 18,
        padding: "0 4px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-sm)",
        background: "var(--aw-bg-fill-light)",
        color: "var(--aw-text-tertiary)",
        fontFamily: "var(--aw-font-sans)",
        fontSize: "var(--aw-font-size-xs)",
        fontWeight: "var(--aw-font-weight-medium)",
        lineHeight: "var(--aw-line-height-xs)",
        whiteSpace: "nowrap",
      }}
    >
      {display}
    </kbd>
  );
}
