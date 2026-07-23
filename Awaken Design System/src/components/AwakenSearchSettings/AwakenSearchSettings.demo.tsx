import { useState } from "react";
import { AwakenSettingsSectionPreview } from "../AwakenSettings/AwakenSettingsSectionPreview";
import {
  AwakenSearchSettings,
  type AwakenSearchProviderItem,
} from "./AwakenSearchSettings";
import { AwakenSearchSettingsSample } from "./AwakenSearchSettings.states";

export function AwakenSearchSettingsDemo() {
  const [providers, setProviders] = useState<AwakenSearchProviderItem[]>(
    AwakenSearchSettingsSample.providers,
  );
  return (
    <section className="ads-component-demo" aria-label="搜索设置 AwakenSearchSettings 组件演示">
      <header className="ads-component-demo-header">
        <h2>搜索设置 <code>AwakenSearchSettings</code></h2>
        <p>搜索渠道设置页；仅替换为无副作用示例数据。</p>
      </header>
      <AwakenSettingsSectionPreview activeSectionId="search" sectionTitle="搜索">
        <AwakenSearchSettings
          {...AwakenSearchSettingsSample}
          providers={providers}
          onToggleProvider={(id) => setProviders((current) => current.map((provider) => (
            provider.id === id ? { ...provider, enabled: !provider.enabled } : provider
          )))}
        />
      </AwakenSettingsSectionPreview>
    </section>
  );
}
