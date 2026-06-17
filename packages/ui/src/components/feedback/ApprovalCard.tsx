import { useEffect, useId } from "react";
import type { ApprovalRiskTone } from "../../types";
import { ShieldCheck, AlertTriangle, AlertOctagon } from "lucide-react";

type ApprovalCardProps = {
  title: string;
  description?: string;
  riskTone?: ApprovalRiskTone;
  approveLabel?: string;
  rejectLabel?: string;
  onApprove?: () => void;
  onReject?: () => void;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
};

type ToneConfig = {
  Icon: typeof ShieldCheck;
  color: string;
  softBg: string;
  softBorder: string;
};

const toneConfig: Record<ApprovalRiskTone, ToneConfig> = {
  safe: {
    Icon: ShieldCheck,
    color: "var(--aw-success)",
    softBg: "var(--aw-success-soft-bg)",
    softBorder: "var(--aw-success-soft-border)",
  },
  warning: {
    Icon: AlertTriangle,
    color: "var(--aw-warning)",
    softBg: "var(--aw-warning-soft-bg)",
    softBorder: "var(--aw-warning-soft-border)",
  },
  danger: {
    Icon: AlertOctagon,
    color: "var(--aw-error)",
    softBg: "var(--aw-error-soft-bg)",
    softBorder: "var(--aw-error-soft-border)",
  },
};

export function ApprovalCard({
  title,
  description,
  riskTone = "warning",
  approveLabel = "允许",
  rejectLabel = "拒绝",
  onApprove,
  onReject,
  open,
  onOpenChange,
}: ApprovalCardProps) {
  const titleId = useId();
  const descId = useId();
  const tone = toneConfig[riskTone];
  const { Icon } = tone;

  const handleApprove = () => {
    onApprove?.();
    onOpenChange?.(false);
  };

  const handleReject = () => {
    onReject?.();
    onOpenChange?.(false);
  };

  useEffect(() => {
    if (!open) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        handleReject();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [open, onReject, onOpenChange]);

  if (!open) return null;

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
        background: "rgba(0, 0, 0, 0.28)",
      }}
      onClick={(e) => {
        if (e.currentTarget === e.target) {
          handleReject();
        }
      }}
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
        tabIndex={-1}
        style={{
          width: "100%",
          maxWidth: "400px",
          padding: "20px",
          border: "1px solid",
          borderColor: tone.softBorder,
          borderRadius: "var(--aw-radius-lg)",
          background: "var(--aw-bg-white)",
          boxShadow: "var(--aw-shadow-menu)",
        }}
      >
        <div
          style={{
            display: "flex",
            alignItems: "flex-start",
            gap: "12px",
            marginBottom: "16px",
          }}
        >
          <div
            style={{
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              width: "36px",
              height: "36px",
              flex: "0 0 auto",
              borderRadius: "var(--aw-radius-md)",
              background: tone.softBg,
              color: tone.color,
            }}
          >
            <Icon size={20} strokeWidth={1.85} />
          </div>
          <div style={{ minWidth: 0 }}>
            <h2
              id={titleId}
              className="aw-truncate"
              style={{
                margin: "0 0 4px",
                fontSize: "var(--aw-font-size-base)",
                lineHeight: "var(--aw-line-height-base)",
                fontWeight: "var(--aw-font-weight-bold)",
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
            onClick={handleReject}
            style={{
              height: "34px",
              padding: "0 14px",
              border: "1px solid var(--aw-border-main)",
              borderRadius: "var(--aw-radius-sm)",
              background: "var(--aw-bg-white)",
              color: "var(--aw-text-primary)",
              fontSize: "var(--aw-font-size-sm)",
              fontWeight: "var(--aw-font-weight-bold)",
              cursor: "pointer",
              transition:
                "background-color 160ms ease, color 160ms ease, transform 160ms ease",
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.background = "var(--aw-bg-fill-light)";
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.background = "var(--aw-bg-white)";
            }}
          >
            {rejectLabel}
          </button>
          <button
            type="button"
            onClick={handleApprove}
            style={{
              height: "34px",
              padding: "0 14px",
              border: "1px solid",
              borderColor: tone.color,
              borderRadius: "var(--aw-radius-sm)",
              background: tone.color,
              color: "var(--aw-text-onblack)",
              fontSize: "var(--aw-font-size-sm)",
              fontWeight: "var(--aw-font-weight-bold)",
              cursor: "pointer",
              transition:
                "filter 160ms ease, transform 160ms ease",
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.filter = "brightness(0.92)";
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.filter = "none";
            }}
          >
            {approveLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
