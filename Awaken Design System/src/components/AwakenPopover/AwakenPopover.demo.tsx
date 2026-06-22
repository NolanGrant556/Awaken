import "./AwakenPopover.css";
import type { ComponentType } from "react";
import { AwakenPopover } from "./AwakenPopover";
import { AwakenPopoverStates } from "./AwakenPopover.states";

export function AwakenPopoverDemo() {
  const Component = AwakenPopover as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPopoverStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenPopover demo">
      <header className="ads-component-demo-header">
        <h2>AwakenPopover</h2>
        <p>Standalone observable states for AwakenPopover.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenPopoverStates.map((state) => (
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
