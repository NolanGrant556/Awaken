import "./AwakenSelectableCard.css";
import type { ButtonHTMLAttributes, ReactNode } from "react";

export type AwakenSelectableCardProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "title"> & {
  selected?: boolean;
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  /** Renders the source as an inline chip that takes only the width it needs.
   * Use it where the source is a secondary detail rather than the choice the
   * layout is built around. The chip has room for the title alone, so it drops
   * `description` and shows selection through its own fill instead of a radio
   * dot. */
  compact?: boolean;
  onSelect?: () => void;
};

export function AwakenSelectableCard({
  selected = false,
  icon,
  title,
  description,
  compact = false,
  onSelect,
  disabled,
  ...props
}: AwakenSelectableCardProps) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      data-selected={selected ? "true" : "false"}
      data-compact={compact ? "true" : "false"}
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
        {description && !compact ? (
          <span className="aw-selectable-card__description">{description}</span>
        ) : null}
      </span>
      {compact ? null : (
        <span className="aw-selectable-card__indicator" aria-hidden="true" />
      )}
    </button>
  );
}
