import { AwakenSettingsNav } from "./AwakenSettingsNav";
import { AwakenSettingsNavStates } from "./AwakenSettingsNav.states";

export function AwakenSettingsNavDemo() {
  return (
    <section className="ads-component-demo" aria-label="设置导航 AwakenSettingsNav 组件演示">
      <header className="ads-component-demo-header">
        <h2>设置导航 <code>AwakenSettingsNav</code></h2>
        <p>承载各设置分区入口与固定末项「已归档的聊天」（含「已归档」分组标题）。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSettingsNavStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas" style={{ width: 148 }}>
              <AwakenSettingsNav {...state.props} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
