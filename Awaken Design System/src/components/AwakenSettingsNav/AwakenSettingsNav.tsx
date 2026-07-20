import "./AwakenSettingsNav.css";

export type AwakenSettingsNavItem = {
  id: string;
  label: string;
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
  ariaLabel = "Settings sections",
  onSelect,
}: AwakenSettingsNavProps) {
  return (
    <nav className="settings-dialog-nav" role="tablist" aria-label={ariaLabel}>
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          className={[
            "settings-dialog-nav-item",
            item.id === activeId ? "settings-dialog-nav-item-active" : "",
          ].filter(Boolean).join(" ")}
          role="tab"
          aria-selected={item.id === activeId}
          disabled={item.disabled}
          onClick={() => onSelect?.(item.id)}
        >
          {item.label}
        </button>
      ))}
    </nav>
  );
}
