import "./AwakenWorkbenchChrome.css";
import type { ComponentType } from "react";
import { AwakenWorkbenchChrome } from "./AwakenWorkbenchChrome";
import { AwakenWorkbenchChromeStates } from "./AwakenWorkbenchChrome.states";

export function AwakenWorkbenchChromeDemo() {
  const Component = AwakenWorkbenchChrome as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenWorkbenchChromeStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="工作台框架 AwakenWorkbenchChrome 组件演示">
      <header className="ads-component-demo-header">
        <h2>工作台框架 <code>AwakenWorkbenchChrome</code></h2>
        <p>用于承载 Workbench 顶部的侧栏切换、新任务和右侧状态控件。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenWorkbenchChromeStates.map((state) => (
          <article key={state.id} className="ads-component-demo-state">
            <h3>{state.label}</h3>
            <div className="ads-component-demo-canvas">
              <Component {...(defaultProps as Record<string, unknown>)} {...(state.props as Record<string, unknown>)} />
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}
