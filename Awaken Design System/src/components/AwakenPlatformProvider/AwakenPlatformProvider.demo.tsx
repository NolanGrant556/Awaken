import "./AwakenPlatformProvider.css";
import type { ComponentType } from "react";
import { AwakenPlatformProvider } from "./AwakenPlatformProvider";
import { AwakenPlatformProviderStates } from "./AwakenPlatformProvider.states";

export function AwakenPlatformProviderDemo() {
  const Component = AwakenPlatformProvider as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPlatformProviderStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="AwakenPlatformProvider demo">
      <header className="ads-component-demo-header">
        <h2>AwakenPlatformProvider</h2>
        <p>Standalone observable states for AwakenPlatformProvider.</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenPlatformProviderStates.map((state) => (
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
