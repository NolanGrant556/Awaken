import "./AwakenThinkingBlock.css";
import type { ComponentType } from "react";
import { AwakenThinkingBlock } from "./AwakenThinkingBlock";
import { AwakenThinkingBlockStates } from "./AwakenThinkingBlock.states";

export function AwakenThinkingBlockDemo() {
  const Component = AwakenThinkingBlock as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenThinkingBlockStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="思考块 AwakenThinkingBlock 组件演示">
      <header className="ads-component-demo-header">
        <h2>思考块 <code>AwakenThinkingBlock</code></h2>
        <p>用于展示模型正在思考或已完成思考的一段可滚动推理文本。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenThinkingBlockStates.map((state) => (
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
