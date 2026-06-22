import "./AwakenInspectorTabs.css";
import type { ComponentType } from "react";
import { AwakenInspectorTabs } from "./AwakenInspectorTabs";
import { AwakenInspectorTabsStates } from "./AwakenInspectorTabs.states";

export function AwakenInspectorTabsDemo() {
  const Component = AwakenInspectorTabs as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenInspectorTabsStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenInspectorTabs demo">
      <header className="ads-component-demo-header">
        <h2>AwakenInspectorTabs</h2>
        <p>Standalone observable states for AwakenInspectorTabs.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenInspectorTabsStates.map((state) => (
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
