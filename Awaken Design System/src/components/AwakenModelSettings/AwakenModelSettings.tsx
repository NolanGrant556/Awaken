import "./AwakenModelSettings.css";
import type { ReactNode } from "react";
import {
  AwakenSettingsTabs,
  type AwakenSettingsTabItem,
} from "../AwakenSettingsTabs";

export type AwakenModelSettingsProps = {
  tabs: AwakenSettingsTabItem[];
  activeTabId: string;
  views: AwakenSettingsTabItem[];
  activeViewId: string;
  title: string;
  description?: ReactNode;
  message?: ReactNode;
  children: ReactNode;
  onSelectTab?: (id: string) => void;
  onSelectView?: (id: string) => void;
};

export function AwakenModelSettings({
  tabs,
  activeTabId,
  views,
  activeViewId,
  title,
  description,
  message,
  children,
  onSelectTab,
  onSelectView,
}: AwakenModelSettingsProps) {
  return (
    <section className="aw-model-settings" aria-label="模型">
      <aside className="aw-model-settings__rail" aria-label="模型接入类型">
        <AwakenSettingsTabs
          items={tabs}
          activeId={activeTabId}
          ariaLabel="模型接入类型"
          onSelect={onSelectTab}
        />
      </aside>
      <div className="aw-model-settings__workspace">
        <header className="aw-model-settings__header">
          <div className="aw-model-settings__heading">
            <h2>{title}</h2>
            {description ? <p>{description}</p> : null}
          </div>
          <AwakenSettingsTabs
            items={views}
            activeId={activeViewId}
            ariaLabel={`${title}设置内容`}
            onSelect={onSelectView}
          />
        </header>
        {message ? <div className="model-settings-message">{message}</div> : null}
        <div className="aw-model-settings__content">{children}</div>
      </div>
    </section>
  );
}
