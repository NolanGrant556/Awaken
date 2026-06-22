import "./AwakenDropdown.css";
import type { ComponentType } from "react";
import { AwakenDropdown } from "./AwakenDropdown";
import { AwakenDropdownStates } from "./AwakenDropdown.states";

export function AwakenDropdownDemo() {
  const Component = AwakenDropdown as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenDropdownStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="下拉菜单 AwakenDropdown 组件演示">
      <header className="ads-component-demo-header">
        <h2>下拉菜单 <code>AwakenDropdown</code></h2>
        <p>用于把触发器和浮层内容组合成下拉容器，承载模型、视图或轻量操作菜单。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenDropdownStates.map((state) => (
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
