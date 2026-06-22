import "./AwakenAvatar.css";
import type { ComponentType } from "react";
import { AwakenAvatar } from "./AwakenAvatar";
import { AwakenAvatarStates } from "./AwakenAvatar.states";

export function AwakenAvatarDemo() {
  const Component = AwakenAvatar as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenAvatarStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenAvatar demo">
      <header className="ads-component-demo-header">
        <h2>AwakenAvatar</h2>
        <p>Standalone observable states for AwakenAvatar.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenAvatarStates.map((state) => (
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
