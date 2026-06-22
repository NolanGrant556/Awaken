import "./AwakenThoughtTrace.css";
import type { ComponentType } from "react";
import { AwakenThoughtTrace } from "./AwakenThoughtTrace";
import { AwakenThoughtTraceStates } from "./AwakenThoughtTrace.states";

export function AwakenThoughtTraceDemo() {
  const Component = AwakenThoughtTrace as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenThoughtTraceStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="思路轨迹 AwakenThoughtTrace 组件演示">
      <header className="ads-component-demo-header">
        <h2>思路轨迹 <code>AwakenThoughtTrace</code></h2>
        <p>用于在执行流中折叠展示单段模型思考轨迹及其状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenThoughtTraceStates.map((state) => (
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
