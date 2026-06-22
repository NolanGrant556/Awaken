import "./AwakenQuickActionChip.css";
import type { ComponentType } from "react";
import { AwakenQuickActionChip } from "./AwakenQuickActionChip";
import { AwakenQuickActionChipStates } from "./AwakenQuickActionChip.states";

export function AwakenQuickActionChipDemo() {
  const Component = AwakenQuickActionChip as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenQuickActionChipStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenQuickActionChip demo">
      <header className="ads-component-demo-header">
        <h2>AwakenQuickActionChip</h2>
        <p>Standalone observable states for AwakenQuickActionChip.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenQuickActionChipStates.map((state) => (
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
