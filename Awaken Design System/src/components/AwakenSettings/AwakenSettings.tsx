import "./AwakenSettings.css";
import type { CSSProperties, ReactNode } from "react";
import { AwakenSettingsNav, type AwakenSettingsNavItem } from "../AwakenSettingsNav";
import { AwakenSettingsTabs, type AwakenSettingsTabItem } from "../AwakenSettingsTabs";

export type AwakenSettingsProps = {
  title?: string;
  navItems: AwakenSettingsNavItem[];
  activeSectionId: string;
  sectionTitle: string;
  tabs?: AwakenSettingsTabItem[];
  activeTabId?: string;
  message?: ReactNode;
  toast?: ReactNode;
  children: ReactNode;
  className?: string;
  style?: CSSProperties;
  onSelectSection: (id: string) => void;
  onSelectTab?: (id: string) => void;
};

export function AwakenSettings({
  title = "设置",
  navItems,
  activeSectionId,
  sectionTitle,
  tabs,
  activeTabId,
  message,
  toast,
  children,
  className,
  style,
  onSelectSection,
  onSelectTab,
}: AwakenSettingsProps) {
  return (
    <div
      className={["settings-dialog-shell", className ?? ""].filter(Boolean).join(" ")}
      style={style}
    >
      {toast ? (
        <div className="model-settings-toast" role="status" aria-live="polite">
          {toast}
        </div>
      ) : null}
      <div className="settings-dialog-sidebar">
        <div className="settings-dialog-sidebar-title">{title}</div>
        <AwakenSettingsNav
          items={navItems}
          activeId={activeSectionId}
          onSelect={onSelectSection}
        />
      </div>

      <div className="model-settings">
        <div className="model-settings-toolbar">
          <h1 className="model-settings-title">{sectionTitle}</h1>
          {tabs?.length ? (
            <AwakenSettingsTabs
              items={tabs}
              activeId={activeTabId}
              onSelect={onSelectTab}
            />
          ) : null}
        </div>
        {message ? <div className="model-settings-message">{message}</div> : null}
        {children}
      </div>
    </div>
  );
}
