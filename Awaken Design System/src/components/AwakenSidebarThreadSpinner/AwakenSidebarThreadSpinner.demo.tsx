import "./AwakenSidebarThreadSpinner.css";
import { AwakenSidebarThreadSpinner } from "./AwakenSidebarThreadSpinner";
import { AwakenSidebarThreadSpinnerStates } from "./AwakenSidebarThreadSpinner.states";

export function AwakenSidebarThreadSpinnerDemo() {
  return (
    <section className="ads-component-demo" aria-label="侧栏线程运行指示 AwakenSidebarThreadSpinner 组件演示">
      <header className="ads-component-demo-header">
        <h2>侧栏线程运行指示 <code>AwakenSidebarThreadSpinner</code></h2>
        <p>用于标记左侧项目树中仍处于 running 或 cancelling 的线程。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSidebarThreadSpinnerStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <AwakenSidebarThreadSpinner {...state.props} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
