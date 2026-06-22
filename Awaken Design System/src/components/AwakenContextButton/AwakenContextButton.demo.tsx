import "./AwakenContextButton.css";
import type { ComponentType } from "react";
import { AwakenContextButton } from "./AwakenContextButton";
import { AwakenContextButtonStates } from "./AwakenContextButton.states";

export function AwakenContextButtonDemo() {
  const Component = AwakenContextButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenContextButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="上下文按钮 AwakenContextButton 组件演示">
      <header className="ads-component-demo-header">
        <h2>上下文按钮 <code>AwakenContextButton</code></h2>
        <p>用于在 composer 工具栏中触发添加上下文文件、目录或外部信息的动作。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenContextButtonStates.map((state) => (
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
