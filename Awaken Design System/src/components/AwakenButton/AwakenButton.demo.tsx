import "./AwakenButton.css";
import type { ComponentType } from "react";
import { AwakenButton } from "./AwakenButton";
import { AwakenButtonStates } from "./AwakenButton.states";

export function AwakenButtonDemo() {
  const Component = AwakenButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenButton demo">
      <header className="ads-component-demo-header">
        <h2>AwakenButton</h2>
        <p>Standalone observable states for AwakenButton.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenButtonStates.map((state) => (
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
