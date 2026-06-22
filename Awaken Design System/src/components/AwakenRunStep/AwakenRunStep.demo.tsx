import "./AwakenRunStep.css";
import type { ComponentType } from "react";
import { AwakenRunStep } from "./AwakenRunStep";
import { AwakenRunStepStates } from "./AwakenRunStep.states";

export function AwakenRunStepDemo() {
  const Component = AwakenRunStep as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenRunStepStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="运行步骤 AwakenRunStep 组件演示">
      <header className="ads-component-demo-header">
        <h2>运行步骤 <code>AwakenRunStep</code></h2>
        <p>用于展示一次运行流程中的单个步骤状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenRunStepStates.map((state) => (
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
