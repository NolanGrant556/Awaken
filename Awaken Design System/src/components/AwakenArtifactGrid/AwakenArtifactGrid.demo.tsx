import "./AwakenArtifactGrid.css";
import type { ComponentType } from "react";
import { AwakenArtifactGrid } from "./AwakenArtifactGrid";
import { AwakenArtifactGridStates } from "./AwakenArtifactGrid.states";

export function AwakenArtifactGridDemo() {
  const Component = AwakenArtifactGrid as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenArtifactGridStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenArtifactGrid demo">
      <header className="ads-component-demo-header">
        <h2>AwakenArtifactGrid</h2>
        <p>Standalone observable states for AwakenArtifactGrid.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenArtifactGridStates.map((state) => (
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
