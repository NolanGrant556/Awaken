import "./AwakenAppShell.css";
import type { ComponentType } from "react";
import { AwakenAppShell } from "./AwakenAppShell";
import { AwakenAppShellStates } from "./AwakenAppShell.states";

export function AwakenAppShellDemo() {
  const Component = AwakenAppShell as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenAppShellStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenAppShell demo">
      <header className="ads-component-demo-header">
        <h2>AwakenAppShell</h2>
        <p>Standalone observable states for AwakenAppShell.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenAppShellStates.map((state) => (
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
