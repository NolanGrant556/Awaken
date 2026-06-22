import "./AwakenProfileButton.css";
import type { ComponentType } from "react";
import { AwakenProfileButton } from "./AwakenProfileButton";
import { AwakenProfileButtonStates } from "./AwakenProfileButton.states";

export function AwakenProfileButtonDemo() {
  const Component = AwakenProfileButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenProfileButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenProfileButton demo">
      <header className="ads-component-demo-header">
        <h2>AwakenProfileButton</h2>
        <p>Standalone observable states for AwakenProfileButton.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenProfileButtonStates.map((state) => (
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
