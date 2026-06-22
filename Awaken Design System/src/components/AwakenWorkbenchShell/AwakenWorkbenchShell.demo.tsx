import "./AwakenWorkbenchShell.css";
import type { ComponentType } from "react";
import { AwakenWorkbenchShell } from "./AwakenWorkbenchShell";
import { AwakenWorkbenchShellStates } from "./AwakenWorkbenchShell.states";

export function AwakenWorkbenchShellDemo() {
  const Component = AwakenWorkbenchShell as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenWorkbenchShellStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenWorkbenchShell demo">
      <header className="ads-component-demo-header">
        <h2>AwakenWorkbenchShell</h2>
        <p>Standalone observable states for AwakenWorkbenchShell.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenWorkbenchShellStates.map((state) => (
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
