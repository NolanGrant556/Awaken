import { AwakenSettingsProviderCard } from "./AwakenSettingsProviderCard";

export function AwakenSettingsProviderCardDemo() {
  return (
    <section className="ads-component-demo" aria-label="模型供应商卡 AwakenSettingsProviderCard 组件演示">
      <header className="ads-component-demo-header">
        <h2>模型供应商卡 <code>AwakenSettingsProviderCard</code></h2>
        <p>展示供应商摘要、状态和可折叠配置容器。</p>
      </header>
      <AwakenSettingsProviderCard name="DeepSeek" defaultExpanded>
        <p style={{ margin: 0, color: "var(--aw-text-secondary)", fontSize: 12 }}>供应商表单由 consumer 组合。</p>
      </AwakenSettingsProviderCard>
    </section>
  );
}
