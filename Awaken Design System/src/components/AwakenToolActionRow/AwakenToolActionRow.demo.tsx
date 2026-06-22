import "./AwakenToolActionRow.css";
import type { ComponentType } from "react";
import { AwakenToolActionRow } from "./AwakenToolActionRow";
import { AwakenToolActionRowStates } from "./AwakenToolActionRow.states";

export function AwakenToolActionRowDemo() {
  const Component = AwakenToolActionRow as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenToolActionRowStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="工具操作行 AwakenToolActionRow 组件演示">
      <header className="ads-component-demo-header">
        <h2>工具操作行 <code>AwakenToolActionRow</code></h2>
        <p>用于在探索步骤中展示一条可选中的工具动作摘要。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenToolActionRowStates.map((state) => (
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
