import "./AwakenComposerInput.css";
import type { ComponentType } from "react";
import { AwakenComposerInput } from "./AwakenComposerInput";
import { AwakenComposerInputStates } from "./AwakenComposerInput.states";

export function AwakenComposerInputDemo() {
  const Component = AwakenComposerInput as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenComposerInputStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenComposerInput demo">
      <header className="ads-component-demo-header">
        <h2>AwakenComposerInput</h2>
        <p>Standalone observable states for AwakenComposerInput.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenComposerInputStates.map((state) => (
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
