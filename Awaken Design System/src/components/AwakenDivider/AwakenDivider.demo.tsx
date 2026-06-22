import "./AwakenDivider.css";
import type { ComponentType } from "react";
import { AwakenDivider } from "./AwakenDivider";
import { AwakenDividerStates } from "./AwakenDivider.states";

export function AwakenDividerDemo() {
  const Component = AwakenDivider as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenDividerStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="分隔线 AwakenDivider 组件演示">
      <header className="ads-component-demo-header">
        <h2>分隔线 <code>AwakenDivider</code></h2>
        <p>用于在面板、表单或叙事区域之间放置横向分隔线，并通过间距控制节奏。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDividerStates.map((state) => (
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
