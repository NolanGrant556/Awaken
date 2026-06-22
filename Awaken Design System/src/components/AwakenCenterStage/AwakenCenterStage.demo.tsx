import "./AwakenCenterStage.css";
import type { ComponentType } from "react";
import { AwakenCenterStage } from "./AwakenCenterStage";
import { AwakenCenterStageStates } from "./AwakenCenterStage.states";

export function AwakenCenterStageDemo() {
  const Component = AwakenCenterStage as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenCenterStageStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenCenterStage demo">
      <header className="ads-component-demo-header">
        <h2>AwakenCenterStage</h2>
        <p>Standalone observable states for AwakenCenterStage.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenCenterStageStates.map((state) => (
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
