import "./AwakenInspectorRail.css";
import type { ComponentType } from "react";
import { AwakenInspectorRail } from "./AwakenInspectorRail";
import { AwakenInspectorRailStates } from "./AwakenInspectorRail.states";

export function AwakenInspectorRailDemo() {
  const Component = AwakenInspectorRail as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenInspectorRailStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenInspectorRail demo">
      <header className="ads-component-demo-header">
        <h2>AwakenInspectorRail</h2>
        <p>Standalone observable states for AwakenInspectorRail.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenInspectorRailStates.map((state) => (
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
