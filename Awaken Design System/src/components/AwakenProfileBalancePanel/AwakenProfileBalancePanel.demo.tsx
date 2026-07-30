import { AwakenProfileBalancePanel } from "./AwakenProfileBalancePanel";
import { AwakenProfileBalancePanelStates } from "./AwakenProfileBalancePanel.states";

export function AwakenProfileBalancePanelDemo() {
  return (
    <section className="ads-component-demo" aria-label="用量双栏面板 AwakenProfileBalancePanel 组件演示">
      <header className="ads-component-demo-header">
        <h2>用量双栏面板 <code>AwakenProfileBalancePanel</code></h2>
        <p>左侧选择已配置 API / Plan，右侧展示 Desktop 提供的用量结果。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenProfileBalancePanelStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenProfileBalancePanel {...state.props} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
