import "./AwakenButton.css";
import type { ComponentType } from "react";
import { AwakenButton } from "./AwakenButton";
import { AwakenButtonStates } from "./AwakenButton.states";

export function AwakenButtonDemo() {
  const Component = AwakenButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="按钮 AwakenButton 组件演示">
      <header className="ads-component-demo-header">
        <h2>按钮 <code>AwakenButton</code></h2>
        <p>用于承载 Awaken 界面里的文字按钮操作，覆盖主次层级、加载和禁用状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenButtonStates.map((state) => (
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
