import "./AwakenModelSettings.css";
import type { ReactNode } from "react";
import {
  AwakenSettingsTabs,
  type AwakenSettingsTabItem,
} from "../AwakenSettingsTabs";

export type AwakenModelSettingsProps = {
  tabs: AwakenSettingsTabItem[];
  activeTabId: string;
  message?: ReactNode;
  children: ReactNode;
  onSelectTab?: (id: string) => void;
};

export function AwakenModelSettings({
  tabs,
  activeTabId,
  message,
  children,
  onSelectTab,
}: AwakenModelSettingsProps) {
  return (
    <section className="aw-model-settings" aria-label="模型">
      <div className="aw-model-settings__tabs">
        <AwakenSettingsTabs
          items={tabs}
          activeId={activeTabId}
          onSelect={onSelectTab}
        />
      </div>
      {message ? <div className="model-settings-message">{message}</div> : null}
      <div className="aw-model-settings__content">{children}</div>
    </section>
  );
}
