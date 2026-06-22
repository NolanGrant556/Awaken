import "./AwakenWindowTitleBar.css";
import type { ComponentType } from "react";
import { AwakenWindowTitleBar } from "./AwakenWindowTitleBar";
import { AwakenWindowTitleBarStates } from "./AwakenWindowTitleBar.states";

export function AwakenWindowTitleBarDemo() {
  const Component = AwakenWindowTitleBar as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenWindowTitleBarStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenWindowTitleBar demo">
      <header className="ads-component-demo-header">
        <h2>AwakenWindowTitleBar</h2>
        <p>Standalone observable states for AwakenWindowTitleBar.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenWindowTitleBarStates.map((state) => (
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
