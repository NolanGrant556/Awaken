import "./AwakenCopyButton.css";
import type { ComponentType } from "react";
import { AwakenCopyButton } from "./AwakenCopyButton";
import { AwakenCopyButtonStates } from "./AwakenCopyButton.states";

export function AwakenCopyButtonDemo() {
  const Component = AwakenCopyButton as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenCopyButtonStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="复制按钮 AwakenCopyButton 组件演示">
      <header className="ads-component-demo-header">
        <h2>复制按钮 <code>AwakenCopyButton</code></h2>
        <p>用于把回答、代码片段或路径复制到剪贴板，并在按钮内反馈复制结果。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenCopyButtonStates.map((state) => (
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
