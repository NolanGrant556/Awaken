import "./AwakenBrandRow.css";
import type { ComponentType } from "react";
import { AwakenBrandRow } from "./AwakenBrandRow";
import { AwakenBrandRowStates } from "./AwakenBrandRow.states";

export function AwakenBrandRowDemo() {
  const Component = AwakenBrandRow as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenBrandRowStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenBrandRow demo">
      <header className="ads-component-demo-header">
        <h2>AwakenBrandRow</h2>
        <p>Standalone observable states for AwakenBrandRow.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenBrandRowStates.map((state) => (
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
