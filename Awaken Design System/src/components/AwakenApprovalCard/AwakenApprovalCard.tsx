import "./AwakenApprovalCard.css";
import { useEffect, useId } from "react";
import type { ApprovalRiskTone } from "../../types";
import { ShieldCheck, AlertTriangle, AlertOctagon } from "lucide-react";

export type AwakenApprovalCardProps = {
  title: string;
  description?: string;
  affectedPaths?: string[];
  patchSummary?: string;
  riskTone?: ApprovalRiskTone;
  approveLabel?: string;
  rejectLabel?: string;
  resolvedDecision?: "approved" | "rejected";
  resolving?: boolean;
  onApprove?: () => void;
  onReject?: () => void;
  open?: boolean;
  inline?: boolean;
  onOpenChange?: (open: boolean) => void;
};

type ToneConfig = {
  AwakenIcon: typeof ShieldCheck;
  color: string;
  softBg: string;
  softBorder: string;
};

const toneConfig: Record<ApprovalRiskTone, ToneConfig> = {
  safe: {
    AwakenIcon: ShieldCheck,
    color: "var(--aw-success)",
    softBg: "var(--aw-success-soft-bg)",
    softBorder: "var(--aw-success-soft-border)",
  },
  warning: {
    AwakenIcon: AlertTriangle,
    color: "var(--aw-warning)",
    softBg: "var(--aw-warning-soft-bg)",
    softBorder: "var(--aw-warning-soft-border)",
  },
  danger: {
    AwakenIcon: AlertOctagon,
    color: "var(--aw-error)",
    softBg: "var(--aw-error-soft-bg)",
    softBorder: "var(--aw-error-soft-border)",
  },
};

export function AwakenApprovalCard({
  title,
  description,
  affectedPaths,
  patchSummary,
  riskTone = "warning",
  approveLabel = "允许",
  rejectLabel = "拒绝",
  resolvedDecision,
  resolving = false,
  onApprove,
  onReject,
  open,
  inline = false,
  onOpenChange,
}: AwakenApprovalCardProps) {
  const titleId = useId();
  const descId = useId();
  const summaryId = useId();
  const pathsId = useId();
  const tone = toneConfig[riskTone];

  const describedBy = [
    description ? descId : undefined,
    patchSummary ? summaryId : undefined,
    affectedPaths && affectedPaths.length > 0 ? pathsId : undefined,
  ]
    .filter(Boolean)
    .join(" ") || undefined;
  const { AwakenIcon } = tone;

  const interactiveDisabled =
    Boolean(resolvedDecision) || resolving || !onApprove || !onReject;

  const handleApprove = () => {
    if (interactiveDisabled) return;
    onApprove?.();
    onOpenChange?.(false);
  };

  const handleReject = () => {
    if (interactiveDisabled) return;
    onReject?.();
    onOpenChange?.(false);
  };

  useEffect(() => {
    if (!open || inline) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        if (!interactiveDisabled) {
          handleReject();
        }
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open, inline, onReject, onOpenChange, interactiveDisabled]);

  if (!open) return null;

  const card = (
    <div
      role={inline ? "group" : "alertdialog"}
      aria-modal={inline ? undefined : "true"}
      aria-labelledby={titleId}
      aria-describedby={describedBy}
      tabIndex={inline ? undefined : -1}
      style={{
        width: "100%",
        minWidth: 0,
        maxWidth: inline ? "100%" : "400px",
        boxSizing: "border-box",
        padding: "16px",
        border: "1px solid",
        borderColor: tone.softBorder,
        borderRadius: "var(--aw-radius-panel)",
        background: "var(--aw-bg-white)",
        boxShadow: inline ? "var(--aw-shadow-raised)" : "var(--aw-shadow-menu)",
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "flex-start",
          gap: "12px",
          marginBottom: "14px",
        }}
      >
        <div
          style={{
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            width: "32px",
            height: "32px",
            flex: "0 0 auto",
            borderRadius: "var(--aw-radius-sm)",
            background: tone.softBg,
            color: tone.color,
          }}
        >
          <AwakenIcon size={18} strokeWidth={1.85} />
        </div>
        <div style={{ minWidth: 0 }}>
          <h2
            id={titleId}
            className="aw-truncate"
            style={{
              margin: "0 0 4px",
              fontSize: "var(--aw-font-size-base)",
              lineHeight: "var(--aw-line-height-base)",
              fontWeight: "var(--aw-font-weight-semibold)",
              color: "var(--aw-text-primary)",
            }}
            title={title}
          >
            {title}
          </h2>
          {description ? (
            <p
              id={descId}
              style={{
                margin: 0,
                fontSize: "var(--aw-font-size-sm)",
                lineHeight: "var(--aw-line-height-sm)",
                color: "var(--aw-text-secondary)",
                wordBreak: "break-word",
              }}
            >
              {description}
            </p>
          ) : null}
          {patchSummary ? (
            <div id={summaryId} style={{ marginTop: "12px" }}>
              <span
                style={{
                  fontSize: "var(--aw-font-size-xs)",
                  lineHeight: "var(--aw-line-height-xs)",
                  color: "var(--aw-text-tertiary)",
                  fontWeight: "var(--aw-font-weight-semibold)",
                  textTransform: "uppercase",
                  letterSpacing: "0.02em",
                }}
              >
                变更摘要
              </span>
              <p
                style={{
                  margin: "4px 0 0",
                  fontSize: "var(--aw-font-size-sm)",
                  lineHeight: "var(--aw-line-height-sm)",
                  color: "var(--aw-text-primary)",
                }}
              >
                {patchSummary}
              </p>
            </div>
          ) : null}
          {affectedPaths && affectedPaths.length > 0 ? (
            <div id={pathsId} style={{ marginTop: "12px" }}>
              <span
                style={{
                  fontSize: "var(--aw-font-size-xs)",
                  lineHeight: "var(--aw-line-height-xs)",
                  color: "var(--aw-text-tertiary)",
                  fontWeight: "var(--aw-font-weight-semibold)",
                  textTransform: "uppercase",
                  letterSpacing: "0.02em",
                }}
              >
                受影响路径
              </span>
              <ul
                style={{
                  margin: "4px 0 0",
                  padding: 0,
                  listStyle: "none",
                  maxHeight: "96px",
                  overflowY: "auto",
                  fontSize: "var(--aw-font-size-xs)",
                  lineHeight: "var(--aw-line-height-xs)",
                  color: "var(--aw-text-secondary)",
                  fontFamily: "var(--aw-font-mono)",
                }}
              >
                {affectedPaths.map((path) => (
                  <li
                    key={path}
                    className="aw-truncate"
                    title={path}
                    style={{
                      padding: "2px 0",
                      borderBottom: "1px solid var(--aw-border-light)",
                    }}
                  >
                    {path}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      </div>

      <div
        style={{
          display: "flex",
          justifyContent: "flex-end",
          gap: "8px",
        }}
      >
        <button
          type="button"
          disabled={interactiveDisabled}
          onClick={handleReject}
          style={{
            height: "28px",
            padding: "0 12px",
            border: "1px solid var(--aw-border-main)",
            borderRadius: "var(--aw-radius-sm)",
            background: "var(--aw-bg-white)",
            color: "var(--aw-text-primary)",
            fontSize: "var(--aw-font-size-base)",
            fontWeight: "var(--aw-font-weight-semibold)",
            cursor: interactiveDisabled ? "default" : "pointer",
            opacity: interactiveDisabled ? 0.5 : 1,
            transition:
              "background-color var(--aw-transition-fast), color var(--aw-transition-fast)",
          }}
          onMouseEnter={(e) => {
            if (!interactiveDisabled) {
              e.currentTarget.style.background = "var(--aw-bg-fill-light)";
            }
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.background = "var(--aw-bg-white)";
          }}
        >
          {resolvedDecision === "rejected" ? "已拒绝" : rejectLabel}
        </button>
        <button
          type="button"
          disabled={interactiveDisabled}
          onClick={handleApprove}
          style={{
            height: "28px",
            padding: "0 12px",
            border: "1px solid",
            borderColor: tone.color,
            borderRadius: "var(--aw-radius-sm)",
            background: tone.color,
            color: "var(--aw-text-onblack)",
            fontSize: "var(--aw-font-size-base)",
            fontWeight: "var(--aw-font-weight-semibold)",
            cursor: interactiveDisabled ? "default" : "pointer",
            opacity: interactiveDisabled ? 0.5 : 1,
            transition:
              "filter 160ms ease, transform 160ms ease",
          }}
          onMouseEnter={(e) => {
            if (!interactiveDisabled) {
              e.currentTarget.style.filter = "brightness(0.92)";
            }
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.filter = "none";
          }}
        >
          {resolvedDecision === "approved" ? "已允许" : approveLabel}
        </button>
      </div>
    </div>
  );

  if (inline) {
    return card;
  }

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 15,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: "24px",
        background: "var(--aw-overlay-backdrop)",
      }}
      onClick={(e) => {
        if (e.currentTarget === e.target && !interactiveDisabled) {
          handleReject();
        }
      }}
    >
      {card}
    </div>
  );
}
