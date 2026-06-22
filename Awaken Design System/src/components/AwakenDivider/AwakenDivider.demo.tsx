import "./AwakenDivider.css";
import type { ComponentType } from "react";
import { AwakenDivider } from "./AwakenDivider";
import { AwakenDividerStates } from "./AwakenDivider.states";

export function AwakenDividerDemo() {
  const Component = AwakenDivider as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenDividerStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenDivider demo">
      <header className="ads-component-demo-header">
        <h2>AwakenDivider</h2>
        <p>Standalone observable states for AwakenDivider.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDividerStates.map((state) => (
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
