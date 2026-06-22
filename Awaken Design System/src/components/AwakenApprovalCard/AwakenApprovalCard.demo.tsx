import "./AwakenApprovalCard.css";
import type { ComponentType } from "react";
import { AwakenApprovalCard } from "./AwakenApprovalCard";
import { AwakenApprovalCardStates } from "./AwakenApprovalCard.states";

export function AwakenApprovalCardDemo() {
  const Component = AwakenApprovalCard as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenApprovalCardStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenApprovalCard demo">
      <header className="ads-component-demo-header">
        <h2>AwakenApprovalCard</h2>
        <p>Standalone observable states for AwakenApprovalCard.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenApprovalCardStates.map((state) => (
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
