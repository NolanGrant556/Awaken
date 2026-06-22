import "./AwakenCenterStage.css";
import type { ComponentType } from "react";
import { AwakenCenterStage } from "./AwakenCenterStage";
import { AwakenCenterStageStates } from "./AwakenCenterStage.states";

export function AwakenCenterStageDemo() {
  const Component = AwakenCenterStage as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenCenterStageStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="中央舞台 AwakenCenterStage 组件演示">
      <header className="ads-component-demo-header">
        <h2>中央舞台 <code>AwakenCenterStage</code></h2>
        <p>用于把起始 composer、空状态或核心任务入口限制在主工作区中心宽度内。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenCenterStageStates.map((state) => (
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
