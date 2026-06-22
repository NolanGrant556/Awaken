import "./AwakenPrimaryNav.css";
import type { ComponentType } from "react";
import { AwakenPrimaryNav } from "./AwakenPrimaryNav";
import { AwakenPrimaryNavStates } from "./AwakenPrimaryNav.states";

export function AwakenPrimaryNavDemo() {
  const Component = AwakenPrimaryNav as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPrimaryNavStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenPrimaryNav demo">
      <header className="ads-component-demo-header">
        <h2>AwakenPrimaryNav</h2>
        <p>Standalone observable states for AwakenPrimaryNav.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenPrimaryNavStates.map((state) => (
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
