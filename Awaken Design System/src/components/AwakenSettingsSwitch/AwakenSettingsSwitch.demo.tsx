import { AwakenSettingsSwitch } from "./AwakenSettingsSwitch";
import { AwakenSettingsSwitchStates } from "./AwakenSettingsSwitch.states";

export function AwakenSettingsSwitchDemo() {
  return (
    <section className="ads-component-demo" aria-label="设置开关 AwakenSettingsSwitch 组件演示">
      <header className="ads-component-demo-header">
        <h2>设置开关 <code>AwakenSettingsSwitch</code></h2>
        <p>用于已配置模型的启用状态，以及模型供应商的能力开关。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSettingsSwitchStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas" style={{ maxWidth: 380 }}>
              <AwakenSettingsSwitch {...state.props} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
