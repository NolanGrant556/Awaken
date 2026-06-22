import "./AwakenConversationList.css";
import type { ComponentType } from "react";
import { AwakenConversationList } from "./AwakenConversationList";
import { AwakenConversationListStates } from "./AwakenConversationList.states";

export function AwakenConversationListDemo() {
  const Component = AwakenConversationList as unknown as ComponentType<Record<string, unknown>>;
  const defaultProps = AwakenConversationListStates[0]?.props ?? {};

  return (
    <section className="ads-component-demo" aria-label="会话列表 AwakenConversationList 组件演示">
      <header className="ads-component-demo-header">
        <h2>会话列表 <code>AwakenConversationList</code></h2>
        <p>用于在侧栏列出任务会话，展示当前选中项、时间标签和任务状态。</p>
      </header>
      <div className="ads-component-demo-grid">
        {AwakenConversationListStates.map((state) => (
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
