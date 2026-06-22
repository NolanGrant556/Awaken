import "./AwakenQuickActionGrid.css";
import type { ComponentType } from "react";
import { AwakenQuickActionGrid } from "./AwakenQuickActionGrid";
import { AwakenQuickActionGridStates } from "./AwakenQuickActionGrid.states";

export function AwakenQuickActionGridDemo() {
  const Component = AwakenQuickActionGrid as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenQuickActionGridStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="快捷动作网格 AwakenQuickActionGrid 组件演示">
      <header className="ads-component-demo-header">
        <h2>快捷动作网格 <code>AwakenQuickActionGrid</code></h2>
        <p>用于排列多个快捷任务建议并在可用宽度内自动换行。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenQuickActionGridStates.map((state) => (
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
