import "./AwakenModelSelector.css";
import type { ComponentType } from "react";
import { AwakenModelSelector } from "./AwakenModelSelector";
import { AwakenModelSelectorStates } from "./AwakenModelSelector.states";

export function AwakenModelSelectorDemo() {
  const Component = AwakenModelSelector as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenModelSelectorStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenModelSelector demo">
      <header className="ads-component-demo-header">
        <h2>AwakenModelSelector</h2>
        <p>Standalone observable states for AwakenModelSelector.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenModelSelectorStates.map((state) => (
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
