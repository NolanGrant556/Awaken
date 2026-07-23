import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import { AwakenPetsSettings } from "./AwakenPetsSettings";
import { AwakenPetsSettingsSample } from "./AwakenPetsSettings.states";

export function AwakenPetsSettingsDemo() {
  return (
    <section className="ads-component-demo" aria-label="宠物设置 AwakenPetsSettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>宠物设置 <code>AwakenPetsSettings</code></h2>
        <p>宠物目录、选择与浮窗动作维持现有组件能力。</p>
      </header>
      <AwakenSettingsSectionPreview activeSectionId="pets" sectionTitle="宠物">
        <AwakenPetsSettings {...AwakenPetsSettingsSample} />
      </AwakenSettingsSectionPreview>
    </section>
  );
}
