import "./AwakenStatusPill.css";
import type { ComponentType } from "react";
import { AwakenStatusPill } from "./AwakenStatusPill";
import { AwakenStatusPillStates } from "./AwakenStatusPill.states";

export function AwakenStatusPillDemo() {
  const Component = AwakenStatusPill as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenStatusPillStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="状态胶囊 AwakenStatusPill 组件演示">
      <header className="ads-component-demo-header">
        <h2>状态胶囊 <code>AwakenStatusPill</code></h2>
        <p>用于在侧栏、工具栏或运行摘要中展示简短的语义状态标签。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenStatusPillStates.map((state) => (
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
