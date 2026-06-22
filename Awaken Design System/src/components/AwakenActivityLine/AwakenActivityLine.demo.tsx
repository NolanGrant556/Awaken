import "./AwakenActivityLine.css";
import type { ComponentType } from "react";
import { AwakenActivityLine } from "./AwakenActivityLine";
import { AwakenActivityLineStates } from "./AwakenActivityLine.states";

export function AwakenActivityLineDemo() {
  const Component = AwakenActivityLine as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenActivityLineStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenActivityLine demo">
      <header className="ads-component-demo-header">
        <h2>AwakenActivityLine</h2>
        <p>Standalone observable states for AwakenActivityLine.</p>
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
