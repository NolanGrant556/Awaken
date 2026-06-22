import "./AwakenExploringGroup.css";
import type { ComponentType } from "react";
import { AwakenExploringGroup } from "./AwakenExploringGroup";
import { AwakenExploringGroupStates } from "./AwakenExploringGroup.states";

export function AwakenExploringGroupDemo() {
  const Component = AwakenExploringGroup as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenExploringGroupStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="探索分组 AwakenExploringGroup 组件演示">
      <header className="ads-component-demo-header">
        <h2>探索分组 <code>AwakenExploringGroup</code></h2>
        <p>用于把探索阶段的多个工具动作归组展示，表达正在分析或已完成的上下文收集。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenExploringGroupStates.map((state) => (
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
