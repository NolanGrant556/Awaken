import { useState } from "react";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import type { AwakenThemeChoiceValue } from "../AwakenThemeChoiceGroup";
import { AwakenAppearanceSettings, type AwakenFontScaleValue } from "./AwakenAppearanceSettings";

export function AwakenAppearanceSettingsDemo() {
  const [value, setValue] = useState<AwakenThemeChoiceValue>("system");
  const [fontScale, setFontScale] = useState<AwakenFontScaleValue>("100");
  return (
    <section className="ads-component-demo" aria-label="外观设置 AwakenAppearanceSettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>外观设置 <code>AwakenAppearanceSettings</code></h2>
        <p>沿用 Awaken 当前跟随系统、浅色与黑色主题选择，并提供 100–200% 字号倍率。</p>
      </header>
      <AwakenSettingsSectionPreview activeSectionId="appearance" sectionTitle="外观">
        <AwakenAppearanceSettings
          value={value}
          fontScale={fontScale}
          onValueChange={setValue}
          onFontScaleChange={setFontScale}
        />
      </AwakenSettingsSectionPreview>
    </section>
  );
}
