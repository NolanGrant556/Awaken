import "./AwakenModelSelector.css";
import type { ComponentType } from "react";
import { AwakenModelSelector } from "./AwakenModelSelector";
import { AwakenModelSelectorStates } from "./AwakenModelSelector.states";

export function AwakenModelSelectorDemo() {
  const Component = AwakenModelSelector as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenModelSelectorStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="模型选择器 AwakenModelSelector 组件演示">
      <header className="ads-component-demo-header">
        <h2>模型选择器 <code>AwakenModelSelector</code></h2>
        <p>用于在 Composer 控件区选择当前模型，并通过下拉列表处理可用、禁用和空选项。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenModelSelectorStates.map((state) => (
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
