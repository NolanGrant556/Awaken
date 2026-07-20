import "./AwakenSettingsTabs.css";

export type AwakenSettingsTabItem = {
  id: string;
  label: string;
  disabled?: boolean;
};

export type AwakenSettingsTabsProps = {
  items: AwakenSettingsTabItem[];
  activeId?: string;
  ariaLabel?: string;
  onSelect?: (id: string) => void;
};

export function AwakenSettingsTabs({
  items,
  activeId,
  ariaLabel = "模型功能区",
  onSelect,
}: AwakenSettingsTabsProps) {
  return (
    <div className="model-settings-tabs" role="tablist" aria-label={ariaLabel}>
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          className={[
            "model-settings-tab",
            item.id === activeId ? "model-settings-tab-active" : "",
          ].filter(Boolean).join(" ")}
          role="tab"
          aria-selected={item.id === activeId}
          disabled={item.disabled}
          onClick={() => onSelect?.(item.id)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
