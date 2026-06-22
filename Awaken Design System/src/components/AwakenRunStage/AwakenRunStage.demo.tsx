import "./AwakenRunStage.css";
import type { ComponentType } from "react";
import { AwakenRunStage } from "./AwakenRunStage";
import { AwakenRunStageStates } from "./AwakenRunStage.states";

export function AwakenRunStageDemo() {
  const Component = AwakenRunStage as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenRunStageStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="运行阶段 AwakenRunStage 组件演示">
      <header className="ads-component-demo-header">
        <h2>运行阶段 <code>AwakenRunStage</code></h2>
        <p>用于承载运行工作区的滚动内容和底部输入区。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenRunStageStates.map((state) => (
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
