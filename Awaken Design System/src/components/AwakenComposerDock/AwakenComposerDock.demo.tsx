import "./AwakenComposerDock.css";
import type { ComponentType } from "react";
import { AwakenComposerDock } from "./AwakenComposerDock";
import { AwakenComposerDockStates } from "./AwakenComposerDock.states";

export function AwakenComposerDockDemo() {
  const Component = AwakenComposerDock as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenComposerDockStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenComposerDock demo">
      <header className="ads-component-demo-header">
        <h2>AwakenComposerDock</h2>
        <p>Standalone observable states for AwakenComposerDock.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenComposerDockStates.map((state) => (
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
