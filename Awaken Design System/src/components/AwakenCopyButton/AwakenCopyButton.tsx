import "./AwakenCopyButton.css";
import { AwakenIcon } from "../AwakenIcon";
import { useEffect, useRef, useState, type ButtonHTMLAttributes } from "react";
import { AwakenTooltip } from "../AwakenTooltip";

export type AwakenCopyButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> & {
  text: string;
  label?: string;
  copiedLabel?: string;
  errorLabel?: string;
  iconOnly?: boolean;
  showTooltip?: boolean;
  feedbackDurationMs?: number;
  onCopied?: () => void;
  onCopyError?: (error: unknown) => void;
};

type CopyState = "idle" | "copied" | "error";

async function writeTextToClipboard(text: string): Promise<void> {
  let clipboardError: unknown;

  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch (error) {
      clipboardError = error;
    }
  }

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.top = "-1000px";
  textarea.style.left = "-1000px";
  document.body.appendChild(textarea);
  textarea.select();

  try {
    const copied = document.execCommand("copy");
    if (!copied) {
      throw clipboardError || new Error("Copy command was rejected.");
    }
  } finally {
    document.body.removeChild(textarea);
  }
}

export function AwakenCopyButton({
  text,
  label = "Copy",
  copiedLabel = "Copied",
  errorLabel = "Copy failed",
  iconOnly = false,
  showTooltip = true,
  feedbackDurationMs = 1200,
  onCopied,
  onCopyError,
  disabled,
  ...props
}: AwakenCopyButtonProps) {
  const [copyState, setCopyState] = useState<CopyState>("idle");
  const timerRef = useRef<number | undefined>(undefined);
  const currentLabel =
    copyState === "copied" ? copiedLabel : copyState === "error" ? errorLabel : label;

  useEffect(() => {
    return () => {
      if (timerRef.current !== undefined) {
        window.clearTimeout(timerRef.current);
      }
    };
  }, []);

  async function handleCopy() {
    try {
      await writeTextToClipboard(text);
      setCopyState("copied");
      onCopied?.();
    } catch (error) {
      setCopyState("error");
      onCopyError?.(error);
    }

    if (timerRef.current !== undefined) {
      window.clearTimeout(timerRef.current);
    }
    timerRef.current = window.setTimeout(() => {
      setCopyState("idle");
    }, feedbackDurationMs);
  }

  const icon =
    copyState === "copied" ? (
      <AwakenIcon concept="step-done" size={14} />
    ) : copyState === "error" ? (
      <AwakenIcon concept="error" size={14} />
    ) : (
      <AwakenIcon concept="copy" size={14} />
    );

  const button = (
    <button
      type="button"
      aria-label={currentLabel}
      title={showTooltip ? currentLabel : undefined}
      disabled={disabled || text.length === 0}
      {...props}
      data-copy-state={copyState}
      onClick={(event) => {
        props.onClick?.(event);
        if (event.defaultPrevented) {
          return;
        }
        void handleCopy();
      }}
      style={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        gap: iconOnly ? 0 : "6px",
        minWidth: 0,
        height: "28px",
        width: iconOnly ? "28px" : undefined,
        padding: iconOnly ? 0 : "0 8px",
        border: "1px solid var(--aw-border-main)",
        borderRadius: "var(--aw-radius-control)",
        background: copyState === "copied" ? "var(--aw-success-soft-bg)" : "var(--aw-surface-raised)",
        color:
          copyState === "copied"
            ? "var(--aw-status-success)"
            : copyState === "error"
              ? "var(--aw-status-danger)"
              : "var(--aw-text-muted)",
        cursor: disabled || text.length === 0 ? "not-allowed" : "pointer",
        fontSize: "var(--aw-font-size-xs)",
        fontWeight: "var(--aw-font-weight-semibold)",
        lineHeight: "var(--aw-line-height-xs)",
        opacity: disabled || text.length === 0 ? 0.48 : 1,
        ...props.style,
      }}
    >
      {icon}
      {iconOnly ? null : <span style={{ color: "inherit" }}>{currentLabel}</span>}
    </button>
  );

  return showTooltip ? <AwakenTooltip label={currentLabel}>{button}</AwakenTooltip> : button;
}
