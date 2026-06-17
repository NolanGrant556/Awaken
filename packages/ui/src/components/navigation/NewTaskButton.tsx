import type { ButtonHTMLAttributes, CSSProperties, MouseEvent } from "react";
import type { PlatformKind } from "../../types";
import { Icon } from "../primitives/Icon";
import { Kbd } from "../primitives/Kbd";

type NewTaskButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "onClick"> & {
  onClick?: () => void;
  shortcut?: string;
  platform?: PlatformKind;
};

const buttonStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  height: 40,
  padding: "0 10px",
  border: "1px solid var(--aw-border-light)",
  borderRadius: "var(--aw-radius-md)",
  background: "var(--aw-bg-white)",
  color: "var(--aw-text-primary)",
  fontSize: "var(--aw-font-size-base)",
  lineHeight: "var(--aw-line-height-base)",
  textAlign: "left",
  cursor: "pointer",
  transition: "background-color 160ms ease, border-color 160ms ease",
};

const leftStyle: CSSProperties = {
  display: "inline-flex",
  gap: 8,
  alignItems: "center",
  minWidth: 0,
};

export function NewTaskButton({
  onClick,
  shortcut,
  platform = "macos",
  disabled,
  children = "新建任务",
  ...props
}: NewTaskButtonProps) {
  const handleEnter = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = "var(--aw-bg-fill-light)";
    props.onMouseEnter?.(e);
  };
  const handleLeave = (e: MouseEvent<HTMLButtonElement>) => {
    e.currentTarget.style.background = "var(--aw-bg-white)";
    props.onMouseLeave?.(e);
  };
  const handleDown = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = "var(--aw-bg-fill-main)";
    props.onMouseDown?.(e);
  };
  const handleUp = (e: MouseEvent<HTMLButtonElement>) => {
    if (!disabled) e.currentTarget.style.background = "var(--aw-bg-fill-light)";
    props.onMouseUp?.(e);
  };

  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      aria-label={typeof children === "string" ? children : "新建任务"}
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
        <Icon>
          <path d="M12 5v14" />
          <path d="M5 12h14" />
        </Icon>
        <span>{children}</span>
      </span>
      {shortcut ? <Kbd platform={platform}>{shortcut}</Kbd> : null}
    </button>
  );
}
