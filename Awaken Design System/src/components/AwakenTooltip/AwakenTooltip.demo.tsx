import "./AwakenTooltip.css";
import type { ComponentType } from "react";
import { AwakenTooltip } from "./AwakenTooltip";
import { AwakenTooltipStates } from "./AwakenTooltip.states";

export function AwakenTooltipDemo() {
  const Component = AwakenTooltip as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenTooltipStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenTooltip demo">
      <header className="ads-component-demo-header">
        <h2>AwakenTooltip</h2>
        <p>Standalone observable states for AwakenTooltip.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenTooltipStates.map((state) => (
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
