import "./AwakenAssistantMessage.css";
import type { ComponentType } from "react";
import { AwakenAssistantMessage } from "./AwakenAssistantMessage";
import { AwakenAssistantMessageStates } from "./AwakenAssistantMessage.states";

export function AwakenAssistantMessageDemo() {
  const Component = AwakenAssistantMessage as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenAssistantMessageStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="助手消息 AwakenAssistantMessage 组件演示">
      <header className="ads-component-demo-header">
        <h2>助手消息 <code>AwakenAssistantMessage</code></h2>
        <p>用于渲染助手输出的富 Markdown 回答，包括 GFM、代码块、表格、Mermaid 图表和运行状态样式。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenAssistantMessageStates.map((state) => (
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
