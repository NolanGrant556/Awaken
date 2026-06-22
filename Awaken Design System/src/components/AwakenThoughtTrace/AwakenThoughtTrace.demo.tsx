import "./AwakenThoughtTrace.css";
import type { ComponentType } from "react";
import { AwakenThoughtTrace } from "./AwakenThoughtTrace";
import { AwakenThoughtTraceStates } from "./AwakenThoughtTrace.states";

export function AwakenThoughtTraceDemo() {
  const Component = AwakenThoughtTrace as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenThoughtTraceStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenThoughtTrace demo">
      <header className="ads-component-demo-header">
        <h2>AwakenThoughtTrace</h2>
        <p>Standalone observable states for AwakenThoughtTrace.</p>
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
