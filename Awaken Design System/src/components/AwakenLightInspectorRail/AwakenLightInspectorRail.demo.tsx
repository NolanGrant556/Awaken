import "./AwakenLightInspectorRail.css";
import type { ComponentType } from "react";
import { AwakenLightInspectorRail } from "./AwakenLightInspectorRail";
import { AwakenLightInspectorRailStates } from "./AwakenLightInspectorRail.states";

export function AwakenLightInspectorRailDemo() {
  const Component = AwakenLightInspectorRail as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenLightInspectorRailStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="浅色检查器侧轨 AwakenLightInspectorRail 组件演示">
      <header className="ads-component-demo-header">
        <h2>浅色检查器侧轨 <code>AwakenLightInspectorRail</code></h2>
        <p>用于展示轻量检查器导航、改动计数和当前选中详情。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenLightInspectorRailStates.map((state) => (
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
