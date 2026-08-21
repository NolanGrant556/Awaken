import "./AwakenSettingsNav.css";
import { AwakenIcon } from "../AwakenIcon";
import type { AwakenIconConcept } from "../../icons";

export type AwakenSettingsNavItem = {
  id: string;
  label: string;
  concept?: AwakenIconConcept;
  disabled?: boolean;
};

export type AwakenSettingsNavProps = {
  items: AwakenSettingsNavItem[];
  activeId?: string;
  ariaLabel?: string;
  onSelect?: (id: string) => void;
};

export function AwakenSettingsNav({
  items,
  activeId,
  ariaLabel = "设置分区",
  onSelect,
}: AwakenSettingsNavProps) {
  return (
    <nav className="aw-settings-nav" role="tablist" aria-label={ariaLabel}>
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          className={[
            "aw-settings-nav__item",
            item.id === activeId ? "aw-settings-nav__item--active" : "",
          ].filter(Boolean).join(" ")}
          role="tab"
          aria-selected={item.id === activeId}
          disabled={item.disabled}
          onClick={() => onSelect?.(item.id)}
        >
          {item.concept ? <AwakenIcon concept={item.concept} size={16} /> : null}
          <span>{item.label}</span>
        </button>
      ))}
    </nav>
  );
}
