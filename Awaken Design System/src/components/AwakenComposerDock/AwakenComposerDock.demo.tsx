import "./AwakenComposerDock.css";
import type { ComponentType } from "react";
import { AwakenComposerDock } from "./AwakenComposerDock";
import { AwakenComposerDockStates } from "./AwakenComposerDock.states";

export function AwakenComposerDockDemo() {
  const Component = AwakenComposerDock as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenComposerDockStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="输入停靠栏 AwakenComposerDock 组件演示">
      <header className="ads-component-demo-header">
        <h2>输入停靠栏 <code>AwakenComposerDock</code></h2>
        <p>用于承载固定在工作台底部的 composer 内容，并提供停靠容器边界。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenComposerDockStates.map((state) => (
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
