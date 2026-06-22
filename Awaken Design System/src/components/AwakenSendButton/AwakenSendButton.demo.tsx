import "./AwakenSendButton.css";
import type { ComponentType } from "react";
import { AwakenSendButton } from "./AwakenSendButton";
import { AwakenSendButtonStates } from "./AwakenSendButton.states";

export function AwakenSendButtonDemo() {
  const Component = AwakenSendButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenSendButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenSendButton demo">
      <header className="ads-component-demo-header">
        <h2>AwakenSendButton</h2>
        <p>Standalone observable states for AwakenSendButton.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSendButtonStates.map((state) => (
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
