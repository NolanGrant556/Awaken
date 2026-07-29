import { AwakenSettingsTabs } from "./AwakenSettingsTabs";
import { AwakenSettingsTabsStates } from "./AwakenSettingsTabs.states";

export function AwakenSettingsTabsDemo() {
  return (
    <section className="ads-component-demo" aria-label="设置页内标签 AwakenSettingsTabs 组件演示">
      <header className="ads-component-demo-header">
        <h2>设置页内标签 <code>AwakenSettingsTabs</code></h2>
        <p>用于模型接入类型与“连接配置 / LLM 设置”二级切换。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSettingsTabsStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenSettingsTabs {...state.props} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
