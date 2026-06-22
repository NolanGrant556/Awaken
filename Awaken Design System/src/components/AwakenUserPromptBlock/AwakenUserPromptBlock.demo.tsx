import "./AwakenUserPromptBlock.css";
import type { ComponentType } from "react";
import { AwakenUserPromptBlock } from "./AwakenUserPromptBlock";
import { AwakenUserPromptBlockStates } from "./AwakenUserPromptBlock.states";

export function AwakenUserPromptBlockDemo() {
  const Component = AwakenUserPromptBlock as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenUserPromptBlockStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="用户提示块 AwakenUserPromptBlock 组件演示">
      <header className="ads-component-demo-header">
        <h2>用户提示块 <code>AwakenUserPromptBlock</code></h2>
        <p>用于在执行流中展示用户发起本轮任务的原始提示。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenUserPromptBlockStates.map((state) => (
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
