import "./AwakenDisclosure.css";
import type { ComponentType } from "react";
import { AwakenDisclosure } from "./AwakenDisclosure";
import { AwakenDisclosureStates } from "./AwakenDisclosure.states";

export function AwakenDisclosureDemo() {
  const Component = AwakenDisclosure as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenDisclosureStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="展开折叠 AwakenDisclosure 组件演示">
      <header className="ads-component-demo-header">
        <h2>展开折叠 <code>AwakenDisclosure</code></h2>
        <p>用于折叠和展开一段可选细节内容，适合收纳执行日志、说明和补充信息。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDisclosureStates.map((state) => (
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
