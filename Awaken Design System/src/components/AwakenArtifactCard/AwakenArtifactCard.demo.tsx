import "./AwakenArtifactCard.css";
import type { ComponentType } from "react";
import { AwakenArtifactCard } from "./AwakenArtifactCard";
import { AwakenArtifactCardStates } from "./AwakenArtifactCard.states";

export function AwakenArtifactCardDemo() {
  const Component = AwakenArtifactCard as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenArtifactCardStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenArtifactCard demo">
      <header className="ads-component-demo-header">
        <h2>AwakenArtifactCard</h2>
        <p>Standalone observable states for AwakenArtifactCard.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenArtifactCardStates.map((state) => (
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
