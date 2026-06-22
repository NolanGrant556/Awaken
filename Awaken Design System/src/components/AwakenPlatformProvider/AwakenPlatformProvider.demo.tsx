import "./AwakenPlatformProvider.css";
import type { ComponentType } from "react";
import { AwakenPlatformProvider } from "./AwakenPlatformProvider";
import { AwakenPlatformProviderStates } from "./AwakenPlatformProvider.states";

export function AwakenPlatformProviderDemo() {
  const Component = AwakenPlatformProvider as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPlatformProviderStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="平台提供器 AwakenPlatformProvider 组件演示">
      <header className="ads-component-demo-header">
        <h2>平台提供器 <code>AwakenPlatformProvider</code></h2>
        <p>用于向子组件提供当前平台和快捷键修饰键信息。</p>
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
