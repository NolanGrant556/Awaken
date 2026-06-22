import "./AwakenIconButton.css";
import type { ComponentType } from "react";
import { AwakenIconButton } from "./AwakenIconButton";
import { AwakenIconButtonStates } from "./AwakenIconButton.states";

export function AwakenIconButtonDemo() {
  const Component = AwakenIconButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenIconButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="图标按钮 AwakenIconButton 组件演示">
      <header className="ads-component-demo-header">
        <h2>图标按钮 <code>AwakenIconButton</code></h2>
        <p>用于呈现只有图标的工具按钮，通过 label 提供可访问名称和悬浮提示。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenIconButtonStates.map((state) => (
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
