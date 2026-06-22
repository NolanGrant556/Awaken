import "./AwakenProjectTree.css";
import type { ComponentType } from "react";
import { AwakenProjectTree } from "./AwakenProjectTree";
import { AwakenProjectTreeStates } from "./AwakenProjectTree.states";

export function AwakenProjectTreeDemo() {
  const Component = AwakenProjectTree as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenProjectTreeStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenProjectTree demo">
      <header className="ads-component-demo-header">
        <h2>AwakenProjectTree</h2>
        <p>Standalone observable states for AwakenProjectTree.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenProjectTreeStates.map((state) => (
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
