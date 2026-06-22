import "./AwakenLightInspectorRail.css";
import type { ComponentType } from "react";
import { AwakenLightInspectorRail } from "./AwakenLightInspectorRail";
import { AwakenLightInspectorRailStates } from "./AwakenLightInspectorRail.states";

export function AwakenLightInspectorRailDemo() {
  const Component = AwakenLightInspectorRail as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenLightInspectorRailStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenLightInspectorRail demo">
      <header className="ads-component-demo-header">
        <h2>AwakenLightInspectorRail</h2>
        <p>Standalone observable states for AwakenLightInspectorRail.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenLightInspectorRailStates.map((state) => (
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
