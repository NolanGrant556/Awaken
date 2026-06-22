import "./AwakenExploringGroup.css";
import type { ComponentType } from "react";
import { AwakenExploringGroup } from "./AwakenExploringGroup";
import { AwakenExploringGroupStates } from "./AwakenExploringGroup.states";

export function AwakenExploringGroupDemo() {
  const Component = AwakenExploringGroup as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenExploringGroupStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenExploringGroup demo">
      <header className="ads-component-demo-header">
        <h2>AwakenExploringGroup</h2>
        <p>Standalone observable states for AwakenExploringGroup.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenExploringGroupStates.map((state) => (
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
