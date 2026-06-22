import "./AwakenPromptCard.css";
import type { ComponentType } from "react";
import { AwakenPromptCard } from "./AwakenPromptCard";
import { AwakenPromptCardStates } from "./AwakenPromptCard.states";

export function AwakenPromptCardDemo() {
  const Component = AwakenPromptCard as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPromptCardStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenPromptCard demo">
      <header className="ads-component-demo-header">
        <h2>AwakenPromptCard</h2>
        <p>Standalone observable states for AwakenPromptCard.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenPromptCardStates.map((state) => (
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
