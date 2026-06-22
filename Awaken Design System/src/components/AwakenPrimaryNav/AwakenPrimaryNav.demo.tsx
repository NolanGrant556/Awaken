import "./AwakenPrimaryNav.css";
import type { ComponentType } from "react";
import { AwakenPrimaryNav } from "./AwakenPrimaryNav";
import { AwakenPrimaryNavStates } from "./AwakenPrimaryNav.states";

export function AwakenPrimaryNavDemo() {
  const Component = AwakenPrimaryNav as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenPrimaryNavStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="主导航 AwakenPrimaryNav 组件演示">
      <header className="ads-component-demo-header">
        <h2>主导航 <code>AwakenPrimaryNav</code></h2>
        <p>用于渲染侧边栏中的一级导航入口和当前选中项。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenPrimaryNavStates.map((state) => (
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
