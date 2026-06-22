import "./AwakenComposer.css";
import type { ComponentType } from "react";
import { AwakenComposer } from "./AwakenComposer";
import { AwakenComposerStates } from "./AwakenComposer.states";

export function AwakenComposerDemo() {
  const Component = AwakenComposer as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenComposerStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="指令编写器 AwakenComposer 组件演示">
      <header className="ads-component-demo-header">
        <h2>指令编写器 <code>AwakenComposer</code></h2>
        <p>用于组合任务输入、上下文添加、Plan/Act 模式、模型选择和发送/停止操作。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenComposerStates.map((state) => (
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
