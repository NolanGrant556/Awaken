import "./AwakenDropdown.css";
import type { ComponentType } from "react";
import { AwakenDropdown } from "./AwakenDropdown";
import { AwakenDropdownStates } from "./AwakenDropdown.states";

export function AwakenDropdownDemo() {
  const Component = AwakenDropdown as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenDropdownStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenDropdown demo">
      <header className="ads-component-demo-header">
        <h2>AwakenDropdown</h2>
        <p>Standalone observable states for AwakenDropdown.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDropdownStates.map((state) => (
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
