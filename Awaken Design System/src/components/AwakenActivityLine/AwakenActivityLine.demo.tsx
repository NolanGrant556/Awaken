import "./AwakenActivityLine.css";
import type { ComponentType } from "react";
import { AwakenActivityLine } from "./AwakenActivityLine";
import { AwakenActivityLineStates } from "./AwakenActivityLine.states";

export function AwakenActivityLineDemo() {
  const Component = AwakenActivityLine as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenActivityLineStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="活动状态行 AwakenActivityLine 组件演示">
      <header className="ads-component-demo-header">
        <h2>活动状态行 <code>AwakenActivityLine</code></h2>
        <p>用于在运行过程、工具调用或任务流中展示单条活动状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenActivityLineStates.map((state) => (
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
