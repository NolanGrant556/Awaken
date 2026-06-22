import "./AwakenToolTraceLine.css";
import type { ComponentType } from "react";
import { AwakenToolTraceLine } from "./AwakenToolTraceLine";
import { AwakenToolTraceLineStates } from "./AwakenToolTraceLine.states";

export function AwakenToolTraceLineDemo() {
  const Component = AwakenToolTraceLine as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenToolTraceLineStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenToolTraceLine demo">
      <header className="ads-component-demo-header">
        <h2>AwakenToolTraceLine</h2>
        <p>Standalone observable states for AwakenToolTraceLine.</p>
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
