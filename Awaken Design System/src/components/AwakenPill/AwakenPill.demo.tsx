import "./AwakenPill.css";
import type { ComponentType } from "react";
import { AwakenPill } from "./AwakenPill";
import { AwakenPillStates } from "./AwakenPill.states";

export function AwakenPillDemo() {
  const Component = AwakenPill as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPillStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="胶囊标签 AwakenPill 组件演示">
      <header className="ads-component-demo-header">
        <h2>胶囊标签 <code>AwakenPill</code></h2>
        <p>用于展示可点击的筛选项、模式项或轻量状态选项。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenPillStates.map((state) => (
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
