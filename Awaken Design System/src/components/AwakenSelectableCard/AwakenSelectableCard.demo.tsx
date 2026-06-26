import "./AwakenSelectableCard.css";
import type { ComponentType } from "react";
import { AwakenSelectableCard } from "./AwakenSelectableCard";
import { AwakenSelectableCardStates } from "./AwakenSelectableCard.states";

export function AwakenSelectableCardDemo() {
  const Component = AwakenSelectableCard as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenSelectableCardStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="选择卡片 AwakenSelectableCard 组件演示">
      <header className="ads-component-demo-header">
        <h2>选择卡片 <code>AwakenSelectableCard</code></h2>
        <p>用于创建项目流程中的来源选择，覆盖选中、禁用和图标说明。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSelectableCardStates.map((state) => (
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
