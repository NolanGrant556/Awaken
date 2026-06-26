import "./AwakenSidebarSectionAction.css";
import type { ComponentType } from "react";
import { AwakenSidebarSectionAction } from "./AwakenSidebarSectionAction";
import { AwakenSidebarSectionActionStates } from "./AwakenSidebarSectionAction.states";

export function AwakenSidebarSectionActionDemo() {
  const Component = AwakenSidebarSectionAction as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenSidebarSectionActionStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="侧栏区块动作 AwakenSidebarSectionAction 组件演示">
      <header className="ads-component-demo-header">
        <h2>侧栏区块动作 <code>AwakenSidebarSectionAction</code></h2>
        <p>用于项目、会话等侧栏分组标题右侧的紧凑图标操作。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSidebarSectionActionStates.map((state) => (
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
