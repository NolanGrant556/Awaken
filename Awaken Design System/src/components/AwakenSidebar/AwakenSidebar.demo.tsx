import "./AwakenSidebar.css";
import type { ComponentType } from "react";
import { AwakenSidebar } from "./AwakenSidebar";
import { AwakenSidebarStates } from "./AwakenSidebar.states";

export function AwakenSidebarDemo() {
  const Component = AwakenSidebar as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenSidebarStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="侧栏 AwakenSidebar 组件演示">
      <header className="ads-component-demo-header">
        <h2>侧栏 <code>AwakenSidebar</code></h2>
        <p>用于组合 Awaken 桌面端的品牌、新建任务、导航、项目和账号入口。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenSidebarStates.map((state) => (
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
