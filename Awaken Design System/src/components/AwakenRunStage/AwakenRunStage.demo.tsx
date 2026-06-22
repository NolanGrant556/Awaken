import "./AwakenRunStage.css";
import type { ComponentType } from "react";
import { AwakenRunStage } from "./AwakenRunStage";
import { AwakenRunStageStates } from "./AwakenRunStage.states";

export function AwakenRunStageDemo() {
  const Component = AwakenRunStage as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenRunStageStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenRunStage demo">
      <header className="ads-component-demo-header">
        <h2>AwakenRunStage</h2>
        <p>Standalone observable states for AwakenRunStage.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenRunStageStates.map((state) => (
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
