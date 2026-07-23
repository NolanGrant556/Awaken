import "./AwakenAppearanceSettings.css";
import {
  AwakenThemeChoiceGroup,
  type AwakenThemeChoiceGroupProps,
} from "../AwakenThemeChoiceGroup";
import type { ReactNode } from "react";

export type AwakenAppearanceSettingsProps = AwakenThemeChoiceGroupProps & {
  message?: ReactNode;
};

export function AwakenAppearanceSettings({ message, ...props }: AwakenAppearanceSettingsProps) {
  return (
    <section className="appearance-settings" aria-label="外观">
      <div className="appearance-settings-row">
        <div className="appearance-settings-copy">
          <h2>主题</h2>
          <p>选择 Awaken 界面的显示方式。</p>
        </div>
        <AwakenThemeChoiceGroup {...props} />
      </div>
      {message ? <div className="model-settings-message">{message}</div> : null}
    </section>
  );
}
