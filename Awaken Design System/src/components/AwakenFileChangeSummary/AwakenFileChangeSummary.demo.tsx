import "./AwakenFileChangeSummary.css";
import type { ComponentType } from "react";
import { AwakenFileChangeSummary } from "./AwakenFileChangeSummary";
import { AwakenFileChangeSummaryStates } from "./AwakenFileChangeSummary.states";

export function AwakenFileChangeSummaryDemo() {
  const Component = AwakenFileChangeSummary as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenFileChangeSummaryStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="文件变更摘要 AwakenFileChangeSummary 组件演示">
      <header className="ads-component-demo-header">
        <h2>文件变更摘要 <code>AwakenFileChangeSummary</code></h2>
        <p>用于汇总一次执行产生的文件改动，展示文件路径、说明和每个文件的处理状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenFileChangeSummaryStates.map((state) => (
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
