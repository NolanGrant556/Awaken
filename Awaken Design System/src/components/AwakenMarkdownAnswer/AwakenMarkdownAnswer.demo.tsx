import "./AwakenMarkdownAnswer.css";
import type { ComponentType } from "react";
import { AwakenMarkdownAnswer } from "./AwakenMarkdownAnswer";
import { AwakenMarkdownAnswerStates } from "./AwakenMarkdownAnswer.states";

export function AwakenMarkdownAnswerDemo() {
  const Component = AwakenMarkdownAnswer as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenMarkdownAnswerStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="Markdown 答案 AwakenMarkdownAnswer 组件演示">
      <header className="ads-component-demo-header">
        <h2>Markdown 答案 <code>AwakenMarkdownAnswer</code></h2>
        <p>用于渲染 Agent 回答中的简化 Markdown 内容，并展示生成中、完成或错误状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenMarkdownAnswerStates.map((state) => (
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
