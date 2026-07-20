import { AwakenProfileBalancePanel } from "./AwakenProfileBalancePanel";
import { AwakenProfileBalancePanelStates } from "./AwakenProfileBalancePanel.states";

export function AwakenProfileBalancePanelDemo() {
  return (
    <section className="ads-component-demo" aria-label="账户额度面板 AwakenProfileBalancePanel 组件演示">
      <header className="ads-component-demo-header">
        <h2>账户额度面板 <code>AwakenProfileBalancePanel</code></h2>
        <p>承接 Desktop 提供的额度数据，并把设置操作交还给 consumer。</p>
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
