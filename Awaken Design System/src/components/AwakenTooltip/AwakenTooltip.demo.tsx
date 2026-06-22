import "./AwakenTooltip.css";
import type { ComponentType } from "react";
import { AwakenTooltip } from "./AwakenTooltip";
import { AwakenTooltipStates } from "./AwakenTooltip.states";

export function AwakenTooltipDemo() {
  const Component = AwakenTooltip as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenTooltipStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="工具提示 AwakenTooltip 组件演示">
      <header className="ads-component-demo-header">
        <h2>工具提示 <code>AwakenTooltip</code></h2>
        <p>用于给图标按钮或紧凑控件补充悬停与聚焦说明。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenTooltipStates.map((state) => (
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
