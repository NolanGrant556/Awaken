import "./AwakenToolTraceGroup.css";
import type { ComponentType } from "react";
import { AwakenToolTraceGroup } from "./AwakenToolTraceGroup";
import { AwakenToolTraceGroupStates } from "./AwakenToolTraceGroup.states";

export function AwakenToolTraceGroupDemo() {
  const Component = AwakenToolTraceGroup as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenToolTraceGroupStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenToolTraceGroup demo">
      <header className="ads-component-demo-header">
        <h2>AwakenToolTraceGroup</h2>
        <p>Standalone observable states for AwakenToolTraceGroup.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenToolTraceGroupStates.map((state) => (
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
