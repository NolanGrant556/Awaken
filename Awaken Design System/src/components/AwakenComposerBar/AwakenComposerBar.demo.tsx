import "./AwakenComposerBar.css";
import type { ComponentType } from "react";
import { AwakenComposerBar } from "./AwakenComposerBar";
import { AwakenComposerBarStates } from "./AwakenComposerBar.states";

export function AwakenComposerBarDemo() {
  const Component = AwakenComposerBar as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenComposerBarStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenComposerBar demo">
      <header className="ads-component-demo-header">
        <h2>AwakenComposerBar</h2>
        <p>Standalone observable states for AwakenComposerBar.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenComposerBarStates.map((state) => (
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
