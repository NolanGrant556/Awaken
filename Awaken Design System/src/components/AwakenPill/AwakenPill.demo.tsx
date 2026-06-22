import "./AwakenPill.css";
import type { ComponentType } from "react";
import { AwakenPill } from "./AwakenPill";
import { AwakenPillStates } from "./AwakenPill.states";

export function AwakenPillDemo() {
  const Component = AwakenPill as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPillStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenPill demo">
      <header className="ads-component-demo-header">
        <h2>AwakenPill</h2>
        <p>Standalone observable states for AwakenPill.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenPillStates.map((state) => (
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
