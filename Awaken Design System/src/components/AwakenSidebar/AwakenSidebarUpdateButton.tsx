import type { CSSProperties, MouseEvent } from "react";
import { AwakenIcon } from "../AwakenIcon";
import type { AwakenIconConcept } from "../../icons";

export type AwakenSidebarUpdateAction = {
  state: "available" | "downloading" | "downloaded" | "error";
  label: string;
  /** 0-100 表示确定进度；undefined 表示总量未知。 */
  progress?: number;
  /** 长期不可用才置 true；下载中保持可点击、指针不变。 */
  disabled?: boolean;
  onClick: () => void;
};

export type AwakenSidebarUpdateButtonProps = {
  action: AwakenSidebarUpdateAction;
};

type UpdateState = AwakenSidebarUpdateAction["state"];

const STATE_CONCEPT: Record<UpdateState, AwakenIconConcept> = {
  available: "update-available",
  downloading: "refresh",
  downloaded: "restart",
  error: "error",
};

const STATE_FOREGROUND: Record<UpdateState, string> = {
  available: "var(--aw-accent)",
  downloading: "var(--aw-accent)",
  downloaded: "var(--aw-accent)",
  error: "var(--aw-error)",
};

const STATE_BORDER: Record<UpdateState, string> = {
  available: "color-mix(in srgb, var(--aw-accent) 24%, transparent)",
  downloading: "var(--aw-border-light)",
  downloaded: "color-mix(in srgb, var(--aw-accent) 45%, transparent)",
  error: "var(--aw-error-soft-border)",
};

const STATE_BACKGROUND: Record<UpdateState, string> = {
  available: "color-mix(in srgb, var(--aw-accent) 8%, transparent)",
  downloading: "transparent",
  downloaded: "color-mix(in srgb, var(--aw-accent) 10%, transparent)",
  error: "var(--aw-error-soft-bg)",
};

const STATE_BACKGROUND_HOVER: Record<UpdateState, string> = {
  available: "color-mix(in srgb, var(--aw-accent) 14%, transparent)",
  downloading: "var(--aw-bg-fill-light)",
  downloaded: "color-mix(in srgb, var(--aw-accent) 16%, transparent)",
  error: "color-mix(in srgb, var(--aw-error) 18%, transparent)",
};

const buttonStyle: CSSProperties = {
  display: "grid",
  gap: 6,
  alignContent: "center",
  width: "100%",
  minHeight: 44,
  padding: "6px 10px",
  border: "1px solid transparent",
  borderRadius: "var(--aw-radius-md)",
  textAlign: "left",
  cursor: "pointer",
  transition: "background-color var(--aw-transition-fast), border-color var(--aw-transition-fast)",
};

const rowStyle: CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 8,
  minWidth: 0,
};

const labelStyle: CSSProperties = {
  overflow: "hidden",
  fontSize: "var(--aw-font-size-sm)",
  lineHeight: "var(--aw-line-height-sm)",
  fontWeight: "var(--aw-font-weight-medium)",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

function clampProgress(progress: number): number {
  return Math.min(100, Math.max(0, progress));
}

export function AwakenSidebarUpdateButton({ action }: AwakenSidebarUpdateButtonProps) {
  const { state, label, progress, disabled, onClick } = action;
    const showProgress =
    state === "downloading" && typeof progress === "number" && Number.isFinite(progress);
  const busy = state === "downloading";

  const handleEnter = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = STATE_BACKGROUND_HOVER[state];
  };
  const handleLeave = (e: MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = STATE_BACKGROUND[state];
  };
  const handleDown = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = STATE_BACKGROUND_HOVER[state];
  };
  const handleUp = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = STATE_BACKGROUND_HOVER[state];
  };

  return (
    <button
      type="button"
      className={`aw-sidebar-update aw-sidebar-update--${state}`}
      aria-label={label}
      aria-busy={busy}
      disabled={disabled}
      onClick={onClick}
      style={{
        ...buttonStyle,
        borderColor: STATE_BORDER[state],
        background: STATE_BACKGROUND[state],
        color: STATE_FOREGROUND[state],
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? 0.45 : 1,
      }}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      onMouseDown={handleDown}
      onMouseUp={handleUp}
    >
      <span style={rowStyle}>
        <AwakenIcon concept={STATE_CONCEPT[state]} size={16} />
        <span style={labelStyle}>{label}</span>
      </span>
      {showProgress ? (
        <span
          className="aw-sidebar-update__track"
          role="progressbar"
          aria-label={label}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(clampProgress(progress))}
        >
          <span
            className="aw-sidebar-update__track-fill"
            style={{ width: `${clampProgress(progress)}%` }}
          />
        </span>
      ) : null}
    </button>
  );
}
