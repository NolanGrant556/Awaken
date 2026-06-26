import "./AwakenSelectableCard.css";
import type { ButtonHTMLAttributes, ReactNode } from "react";

export type AwakenSelectableCardProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "title"> & {
  selected?: boolean;
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  onSelect?: () => void;
};

export function AwakenSelectableCard({
  selected = false,
  icon,
  title,
  description,
  onSelect,
  disabled,
  ...props
}: AwakenSelectableCardProps) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      data-selected={selected ? "true" : "false"}
      disabled={disabled}
      {...props}
      className={`aw-selectable-card${props.className ? ` ${props.className}` : ""}`}
      onClick={(event) => {
        props.onClick?.(event);
        if (!event.defaultPrevented && !disabled) {
          onSelect?.();
        }
      }}
    >
      {icon ? <span className="aw-selectable-card__icon">{icon}</span> : null}
      <span className="aw-selectable-card__content">
        <span className="aw-selectable-card__title">{title}</span>
        {description ? (
          <span className="aw-selectable-card__description">{description}</span>
        ) : null}
      </span>
      <span className="aw-selectable-card__indicator" aria-hidden="true" />
    </button>
  );
}
