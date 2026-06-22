import "./AwakenSidebar.css";
import type { ComponentType } from "react";
import { AwakenSidebar } from "./AwakenSidebar";
import { AwakenSidebarStates } from "./AwakenSidebar.states";

export function AwakenSidebarDemo() {
  const Component = AwakenSidebar as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenSidebarStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenSidebar demo">
      <header className="ads-component-demo-header">
        <h2>AwakenSidebar</h2>
        <p>Standalone observable states for AwakenSidebar.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSidebarStates.map((state) => (
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
