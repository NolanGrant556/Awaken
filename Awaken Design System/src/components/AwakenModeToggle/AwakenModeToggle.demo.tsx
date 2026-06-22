import "./AwakenModeToggle.css";
import type { ComponentType } from "react";
import { AwakenModeToggle } from "./AwakenModeToggle";
import { AwakenModeToggleStates } from "./AwakenModeToggle.states";

export function AwakenModeToggleDemo() {
  const Component = AwakenModeToggle as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenModeToggleStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="模式切换 AwakenModeToggle 组件演示">
      <header className="ads-component-demo-header">
        <h2>模式切换 <code>AwakenModeToggle</code></h2>
        <p>用于在 Composer 中显示并切换 Plan 与 Act 两种执行模式。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenModeToggleStates.map((state) => (
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
