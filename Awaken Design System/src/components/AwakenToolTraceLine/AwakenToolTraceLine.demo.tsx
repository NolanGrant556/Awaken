import "./AwakenToolTraceLine.css";
import type { ComponentType } from "react";
import { AwakenToolTraceLine } from "./AwakenToolTraceLine";
import { AwakenToolTraceLineStates } from "./AwakenToolTraceLine.states";

export function AwakenToolTraceLineDemo() {
  const Component = AwakenToolTraceLine as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenToolTraceLineStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="工具轨迹行 AwakenToolTraceLine 组件演示">
      <header className="ads-component-demo-header">
        <h2>工具轨迹行 <code>AwakenToolTraceLine</code></h2>
        <p>用于在执行流中展示单次工具调用的标题、细节和完成状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenToolTraceLineStates.map((state) => (
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
