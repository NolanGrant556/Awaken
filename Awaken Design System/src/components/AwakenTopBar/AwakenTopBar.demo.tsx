import "./AwakenTopBar.css";
import type { ComponentType } from "react";
import { AwakenTopBar } from "./AwakenTopBar";
import { AwakenTopBarStates } from "./AwakenTopBar.states";

export function AwakenTopBarDemo() {
  const Component = AwakenTopBar as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenTopBarStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenTopBar demo">
      <header className="ads-component-demo-header">
        <h2>AwakenTopBar</h2>
        <p>Standalone observable states for AwakenTopBar.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenTopBarStates.map((state) => (
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
