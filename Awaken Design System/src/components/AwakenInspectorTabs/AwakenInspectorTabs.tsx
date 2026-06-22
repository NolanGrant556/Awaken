import "./AwakenInspectorTabs.css";
import { useRef, type KeyboardEvent } from "react";

export type InspectorTabId = "changes" | "browser" | "terminal" | "files";

export type InspectorTabItem = {
  id: InspectorTabId;
  label: string;
  badge?: number | string;
  disabled?: boolean;
};

export type AwakenInspectorTabsProps = {
  tabs: InspectorTabItem[];
  activeTabId: InspectorTabId;
  onTabChange?: (tabId: InspectorTabId) => void;
};

export function AwakenInspectorTabs({
  tabs,
  activeTabId,
  onTabChange,
}: AwakenInspectorTabsProps) {
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([]);

  function moveToTab(nextIndex: number): void {
    const nextTab = tabs[nextIndex];
    if (!nextTab || nextTab.disabled) {
      return;
    }
    onTabChange?.(nextTab.id);
    window.requestAnimationFrame(() => {
      tabRefs.current[nextIndex]?.focus();
    });
  }

  function handleKeyDown(event: KeyboardEvent<HTMLButtonElement>, index: number): void {
    const enabledIndexes = tabs
      .map((tab, tabIndex) => (tab.disabled ? -1 : tabIndex))
      .filter((tabIndex) => tabIndex >= 0);
    const currentEnabledIndex = enabledIndexes.indexOf(index);
    if (currentEnabledIndex < 0) {
      return;
    }

    if (event.key === "ArrowRight" || event.key === "ArrowDown") {
      event.preventDefault();
      moveToTab(enabledIndexes[(currentEnabledIndex + 1) % enabledIndexes.length]);
      return;
    }

    if (event.key === "ArrowLeft" || event.key === "ArrowUp") {
      event.preventDefault();
      moveToTab(
        enabledIndexes[
          (currentEnabledIndex - 1 + enabledIndexes.length) % enabledIndexes.length
        ],
      );
      return;
    }

    if (event.key === "Home") {
      event.preventDefault();
      moveToTab(enabledIndexes[0]);
      return;
    }

    if (event.key === "End") {
      event.preventDefault();
      moveToTab(enabledIndexes[enabledIndexes.length - 1]);
    }
  }

  return (
    <div
      role="tablist"
      aria-label="Inspector tabs"
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
        gap: "6px",
        minWidth: 0,
      }}
    >
      {tabs.map((tab, index) => {
        const active = tab.id === activeTabId;
        return (
          <button
            key={tab.id}
            ref={(element) => {
              tabRefs.current[index] = element;
            }}
            type="button"
            role="tab"
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            disabled={tab.disabled}
            onClick={() => onTabChange?.(tab.id)}
            onKeyDown={(event) => handleKeyDown(event, index)}
            style={{
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              gap: "6px",
              minWidth: 0,
              height: "32px",
              padding: "0 10px",
              border: "1px solid",
              borderColor: active ? "var(--aw-button-black)" : "var(--aw-border-main)",
              borderRadius: "var(--aw-radius-control)",
              background: active ? "var(--aw-button-black)" : "var(--aw-surface-raised)",
              color: active ? "var(--aw-text-onblack)" : "var(--aw-text-muted)",
              fontSize: "var(--aw-font-size-xs)",
              fontWeight: "var(--aw-font-weight-bold)",
              cursor: tab.disabled ? "default" : "pointer",
              opacity: tab.disabled ? 0.48 : 1,
            }}
          >
            <span className="aw-truncate">{tab.label}</span>
            {tab.badge !== undefined ? (
              <span
                style={{
                  flex: "0 0 auto",
                  minWidth: "18px",
                  padding: "1px 5px",
                  borderRadius: "var(--aw-radius-pill)",
                  background: active ? "color-mix(in srgb, var(--aw-surface-raised) 18%, transparent)" : "var(--aw-bg-fill-light)",
                  color: active ? "var(--aw-text-onblack)" : "var(--aw-text-subtle)",
                  fontSize: "var(--aw-font-size-xs)",
                  lineHeight: "var(--aw-line-height-xs)",
                }}
              >
                {tab.badge}
              </span>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
