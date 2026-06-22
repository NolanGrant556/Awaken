import "./AwakenNarrativeFeed.css";
import type { ComponentType } from "react";
import { AwakenNarrativeFeed } from "./AwakenNarrativeFeed";
import { AwakenNarrativeFeedStates } from "./AwakenNarrativeFeed.states";

export function AwakenNarrativeFeedDemo() {
  const Component = AwakenNarrativeFeed as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenNarrativeFeedStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="叙事动态流 AwakenNarrativeFeed 组件演示">
      <header className="ads-component-demo-header">
        <h2>叙事动态流 <code>AwakenNarrativeFeed</code></h2>
        <p>用于承载运行叙事中的消息、步骤或产物摘要列表。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenNarrativeFeedStates.map((state) => (
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
