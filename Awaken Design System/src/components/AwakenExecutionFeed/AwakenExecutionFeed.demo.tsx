import "./AwakenExecutionFeed.css";
import type { ComponentType } from "react";
import { AwakenExecutionFeed } from "./AwakenExecutionFeed";
import { AwakenExecutionFeedStates } from "./AwakenExecutionFeed.states";

export function AwakenExecutionFeedDemo() {
  const Component = AwakenExecutionFeed as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenExecutionFeedStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="执行动态流 AwakenExecutionFeed 组件演示">
      <header className="ads-component-demo-header">
        <h2>执行动态流 <code>AwakenExecutionFeed</code></h2>
        <p>用于承载 Agent 执行叙事中的一组可见事件或步骤，保持空态与可访问标签。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenExecutionFeedStates.map((state) => (
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
