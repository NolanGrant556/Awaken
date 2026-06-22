import "./AwakenInspectorTabs.css";
import type { ComponentType } from "react";
import { AwakenInspectorTabs } from "./AwakenInspectorTabs";
import { AwakenInspectorTabsStates } from "./AwakenInspectorTabs.states";

export function AwakenInspectorTabsDemo() {
  const Component = AwakenInspectorTabs as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenInspectorTabsStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="检查器标签 AwakenInspectorTabs 组件演示">
      <header className="ads-component-demo-header">
        <h2>检查器标签 <code>AwakenInspectorTabs</code></h2>
        <p>用于在检查器内切换改动、浏览器、终端和文件等固定视图。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenInspectorTabsStates.map((state) => (
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
