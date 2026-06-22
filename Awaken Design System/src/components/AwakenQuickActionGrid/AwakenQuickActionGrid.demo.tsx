import "./AwakenQuickActionGrid.css";
import type { ComponentType } from "react";
import { AwakenQuickActionGrid } from "./AwakenQuickActionGrid";
import { AwakenQuickActionGridStates } from "./AwakenQuickActionGrid.states";

export function AwakenQuickActionGridDemo() {
  const Component = AwakenQuickActionGrid as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenQuickActionGridStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenQuickActionGrid demo">
      <header className="ads-component-demo-header">
        <h2>AwakenQuickActionGrid</h2>
        <p>Standalone observable states for AwakenQuickActionGrid.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenQuickActionGridStates.map((state) => (
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
