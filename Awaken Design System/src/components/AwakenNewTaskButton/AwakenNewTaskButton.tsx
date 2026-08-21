import "./AwakenNewTaskButton.css";
import { AwakenIcon } from "../AwakenIcon";
import type { ButtonHTMLAttributes, CSSProperties, MouseEvent } from "react";

export type AwakenNewTaskButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onClick"> & {
  onClick?: () => void;
};

const buttonStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  height: 40,
  padding: "0 12px",
  border: "1px solid var(--aw-button-black)",
  borderRadius: "var(--aw-radius-sm)",
  background: "var(--aw-button-black)",
  color: "var(--aw-text-onblack)",
  fontSize: "var(--aw-font-size-lg)",
  lineHeight: "var(--aw-line-height-lg)",
  fontWeight: "var(--aw-font-weight-medium)",
  textAlign: "left",
  cursor: "pointer",
  transition: "background-color var(--aw-transition-fast), border-color var(--aw-transition-fast)",
};

const leftStyle: CSSProperties = {
  display: "inline-flex",
  gap: 10,
  alignItems: "center",
  minWidth: 0,
};

export function AwakenNewTaskButton({
  onClick,
  disabled,
  children = "新建任务",
  className,
  ...props
}: AwakenNewTaskButtonProps) {
  const handleEnter = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = "var(--aw-button-black-hover)";
    props.onMouseEnter?.(e);
  };
  const handleLeave = (e: MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = "var(--aw-button-black)";
    props.onMouseLeave?.(e);
  };
  const handleDown = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = "var(--aw-button-black-active)";
    props.onMouseDown?.(e);
  };
  const handleUp = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = "var(--aw-button-black-hover)";
    props.onMouseUp?.(e);
  };

  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-label={typeof children === "string" ? children : "新建任务"}
      className={["aw-new-task-button", className].filter(Boolean).join(" ")}
      style={{
        ...buttonStyle,
        cursor: disabled ? "not-allowed" : "pointer",
        opacity: disabled ? 0.45 : 1,
      }}
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      onMouseDown={handleDown}
      onMouseUp={handleUp}
      {...props}
    >
      <span style={leftStyle}>
        <AwakenIcon concept="add" size={20} />
        <span>{children}</span>
      </span>
    </button>
  );
}
