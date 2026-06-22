import "./AwakenCopyButton.css";
import type { ComponentType } from "react";
import { AwakenCopyButton } from "./AwakenCopyButton";
import { AwakenCopyButtonStates } from "./AwakenCopyButton.states";

export function AwakenCopyButtonDemo() {
  const Component = AwakenCopyButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenCopyButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenCopyButton demo">
      <header className="ads-component-demo-header">
        <h2>AwakenCopyButton</h2>
        <p>Standalone observable states for AwakenCopyButton.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenCopyButtonStates.map((state) => (
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
