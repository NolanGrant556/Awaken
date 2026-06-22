import "./AwakenKbd.css";
import type { ComponentType } from "react";
import { AwakenKbd } from "./AwakenKbd";
import { AwakenKbdStates } from "./AwakenKbd.states";

export function AwakenKbdDemo() {
  const Component = AwakenKbd as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenKbdStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="快捷键标记 AwakenKbd 组件演示">
      <header className="ads-component-demo-header">
        <h2>快捷键标记 <code>AwakenKbd</code></h2>
        <p>用于以内联键帽形式展示快捷键，并按平台补齐 Command 或 Ctrl 前缀。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenKbdStates.map((state) => (
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
