import { useState } from "react";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import type { AwakenThemeChoiceValue } from "../AwakenThemeChoiceGroup";
import { AwakenAppearanceSettings } from "./AwakenAppearanceSettings";

export function AwakenAppearanceSettingsDemo() {
  const [value, setValue] = useState<AwakenThemeChoiceValue>("system");
  return (
    <section className="ads-component-demo" aria-label="外观设置 AwakenAppearanceSettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>外观设置 <code>AwakenAppearanceSettings</code></h2>
        <p>沿用 Awaken 当前跟随系统、浅色与黑色主题选择。</p>
      </header>
      <AwakenSettingsSectionPreview activeSectionId="appearance" sectionTitle="外观">
        <AwakenAppearanceSettings
          value={value}
          onValueChange={setValue}
        />
      </AwakenSettingsSectionPreview>
    </section>
  );
}
