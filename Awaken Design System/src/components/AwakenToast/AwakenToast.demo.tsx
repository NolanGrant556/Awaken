import "./AwakenToast.css";
import type { ComponentType } from "react";
import { AwakenToast } from "./AwakenToast";
import { AwakenToastStates } from "./AwakenToast.states";

export function AwakenToastDemo() {
  const Component = AwakenToast as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenToastStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenToast demo">
      <header className="ads-component-demo-header">
        <h2>AwakenToast</h2>
        <p>Standalone observable states for AwakenToast.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenToastStates.map((state) => (
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
