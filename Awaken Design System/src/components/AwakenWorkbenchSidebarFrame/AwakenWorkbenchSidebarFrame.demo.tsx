import "./AwakenWorkbenchSidebarFrame.css";
import type { ComponentType } from "react";
import { AwakenWorkbenchSidebarFrame } from "./AwakenWorkbenchSidebarFrame";
import { AwakenWorkbenchSidebarFrameStates } from "./AwakenWorkbenchSidebarFrame.states";

export function AwakenWorkbenchSidebarFrameDemo() {
  const Component = AwakenWorkbenchSidebarFrame as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenWorkbenchSidebarFrameStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="工作台侧栏框架 AwakenWorkbenchSidebarFrame 组件演示">
      <header className="ads-component-demo-header">
        <h2>工作台侧栏框架 <code>AwakenWorkbenchSidebarFrame</code></h2>
        <p>用于给 Workbench 侧栏内容提供统一的外层框架。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenWorkbenchSidebarFrameStates.map((state) => (
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
