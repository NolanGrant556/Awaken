import "./AwakenToolActionRow.css";
import type { ComponentType } from "react";
import { AwakenToolActionRow } from "./AwakenToolActionRow";
import { AwakenToolActionRowStates } from "./AwakenToolActionRow.states";

export function AwakenToolActionRowDemo() {
  const Component = AwakenToolActionRow as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenToolActionRowStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenToolActionRow demo">
      <header className="ads-component-demo-header">
        <h2>AwakenToolActionRow</h2>
        <p>Standalone observable states for AwakenToolActionRow.</p>
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
