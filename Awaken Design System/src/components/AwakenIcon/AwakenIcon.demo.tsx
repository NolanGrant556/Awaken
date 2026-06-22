import "./AwakenIcon.css";
import type { ComponentType } from "react";
import { AwakenIcon } from "./AwakenIcon";
import { AwakenIconStates } from "./AwakenIcon.states";

export function AwakenIconDemo() {
  const Component = AwakenIcon as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenIconStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenIcon demo">
      <header className="ads-component-demo-header">
        <h2>AwakenIcon</h2>
        <p>Standalone observable states for AwakenIcon.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenIconStates.map((state) => (
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
